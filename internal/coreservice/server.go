// Package coreservice owns the OpenWrt core and firewall independently of the
// replaceable web backend. Its root-only socket accepts no executable or file paths.
package coreservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/localhost-copilot/clashcube/internal/coremgr"
	"github.com/localhost-copilot/clashcube/internal/router"
	"github.com/localhost-copilot/clashcube/internal/runtimecfg"
	"github.com/localhost-copilot/clashcube/internal/settings"
	"github.com/localhost-copilot/clashcube/internal/webui"
	"go.yaml.in/yaml/v3"
)

const Socket = "/var/run/clashcube-core/control.sock"

type record struct {
	Running    bool                  `json:"running"`
	Controller runtimecfg.Controller `json:"controller"`
}

type Snapshot struct {
	Initialized bool                  `json:"initialized"`
	Status      coremgr.Status        `json:"status"`
	Error       string                `json:"error,omitempty"`
	Version     string                `json:"version"`
	Controller  runtimecfg.Controller `json:"controller"`
}

type supervisor struct {
	mu             sync.Mutex
	core           *coremgr.Manager
	guard          *router.Guard
	path           string
	config         string
	version        string
	initialized    bool
	startupErr     string
	restorePending *runtimecfg.Controller
}

func Serve(ctx context.Context, home, coreHome, version string) error {
	if err := os.MkdirAll(filepath.Join(home, "core"), 0700); err != nil {
		return err
	}
	s := &supervisor{path: filepath.Join(home, "core-service.json"), config: filepath.Join(home, "core/runtime.yaml"), version: version}
	s.core = coremgr.New(&coremgr.LocalRunner{}, coreHome, s.config)
	s.guard = router.NewGuard(home, s.core.Client)
	s.core.OnChange(s.guard.Notify)
	s.core.OnLog(func(line string) { log.Print(line) })
	l, err := webui.Listen(Socket)
	if err != nil {
		return err
	}
	defer l.Close()
	defer func() {
		// A handler may still be unwinding after HTTP shutdown times out.
		// Serialize final cleanup with Start/Stop so it cannot orphan a child.
		s.mu.Lock()
		defer s.mu.Unlock()
		_, _ = s.guard.Pause()
		_ = s.core.Stop()
	}()
	var saved record
	if body, err := os.ReadFile(s.path); err == nil {
		if err := json.Unmarshal(body, &saved); err != nil {
			return err
		}
		s.initialized = true
	} else if !os.IsNotExist(err) {
		return err
	}
	if _, err := s.guard.Reconcile(); err != nil {
		return err
	}
	// A router boot honors AutoStart; an upgrade/respawn preserves current
	// intent. The init boot marker is consumed once, rather than inherited in
	// the respawn environment after the user explicitly stops the core.
	boot := filepath.Join(filepath.Dir(Socket), "boot")
	if _, err := os.Stat(boot); err == nil {
		if err := os.Remove(boot); err != nil {
			return err
		}
		saved, err = s.bootRecord(settings.Load().AutoStart, saved)
		if err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if saved.Running {
		s.restorePending = &saved.Controller
		if err := s.start(saved.Controller, true); err != nil {
			s.startupErr = err.Error()
			log.Printf("restore core: %v", err)
		} else {
			s.restorePending = nil
		}
	}
	watchCtx, cancelWatch := context.WithCancel(ctx)
	defer cancelWatch()
	go s.guard.Watch(watchCtx)
	go router.WatchGEO(watchCtx, home, s.core.Client)
	go s.retryRestore(watchCtx)
	server := &http.Server{Handler: webui.NewRPCHandler(map[string]any{
		"Core/Status": s.Status, "Core/Start": s.Start, "Core/Stop": s.Stop, "Core/Test": s.Test,
		"Firewall/Get": s.guard.Get, "Firewall/Pause": s.guard.Pause,
		"Firewall/Resume": s.guard.Resume, "Firewall/Reconcile": s.guard.Reconcile,
	}), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second}
	stopped := make(chan error, 1)
	go func() { stopped <- server.Serve(l) }()
	select {
	case err := <-stopped:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdown); err != nil {
		return server.Close()
	}
	return nil
}

func (s *supervisor) Status() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	status, message := s.core.Status()
	if message == "" {
		message = s.startupErr
	}
	return Snapshot{Initialized: s.initialized, Status: status, Error: message, Version: s.version, Controller: s.core.Controller()}
}

func (s *supervisor) save(r record) error {
	body, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.path+".tmp", body, 0600); err != nil {
		return err
	}
	if err := os.Rename(s.path+".tmp", s.path); err != nil {
		return err
	}
	s.initialized = true
	return nil
}

func (s *supervisor) Start(ctl runtimecfg.Controller) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restorePending = nil
	return s.start(ctl, false)
}

// start is serialized by mu, including boot retries, so an explicit Stop can
// cancel the pending restore without racing a delayed automatic Start.
func (s *supervisor) start(ctl runtimecfg.Controller, restoring bool) error {
	if status, _ := s.core.Status(); status == coremgr.Running {
		if s.core.Controller() != ctl {
			return errors.New("core already runs with another controller")
		}
		return nil
	}
	// The web process only supplies credentials for the runtime it just built.
	// Nothing here accepts an arbitrary command, data directory, or config path.
	addr, err := netip.ParseAddrPort(ctl.Addr)
	if err != nil || addr.Addr() != netip.MustParseAddr("127.0.0.1") || addr.Port() == 0 || ctl.Secret == "" {
		return errors.New("core controller must be authenticated IPv4 loopback")
	}
	config, err := s.runtimeController()
	if err != nil {
		return err
	}
	if config != ctl {
		return errors.New("controller does not match the validated runtime")
	}
	if err := s.core.Test(); err != nil {
		return err
	}
	if err := s.save(record{Running: true, Controller: ctl}); err != nil {
		return err
	}
	s.core.SetController(ctl)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := s.core.Start(ctx); err != nil {
		// Boot may need network access to refill a RAM cache. Preserve intent
		// across supervisor crashes while retrying that automatic restoration.
		return errors.Join(err, s.save(record{Running: restoring, Controller: ctl}))
	}
	s.startupErr = ""
	// The web configuration transaction may have paused the guard; do not release
	// that pause here. A normal start restores the persisted desired state.
	_, _ = s.guard.Reconcile()
	return nil
}

func (s *supervisor) runtimeController() (runtimecfg.Controller, error) {
	body, err := os.ReadFile(s.config)
	if err != nil {
		return runtimecfg.Controller{}, err
	}
	var config struct {
		Addr   string `yaml:"external-controller"`
		Secret string `yaml:"secret"`
	}
	err = yaml.Unmarshal(body, &config)
	return runtimecfg.Controller{Addr: config.Addr, Secret: config.Secret}, err
}

func (s *supervisor) bootRecord(autoStart bool, saved record) (record, error) {
	if !autoStart {
		saved.Running = false
		if s.initialized {
			return saved, s.save(saved)
		}
		return saved, nil
	}
	ctl, err := s.runtimeController()
	if os.IsNotExist(err) {
		// Fresh installation: management must build and validate the first runtime.
		s.initialized = false
		return record{}, nil
	}
	if err != nil {
		return record{}, err
	}
	running := record{Running: true, Controller: ctl}
	// Persist boot intent before validation can block on missing GEO downloads.
	// A supervisor respawn must keep retrying even if the last session was stopped.
	return running, s.save(running)
}

func (s *supervisor) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restorePending = nil
	if _, err := s.guard.Pause(); err != nil {
		return err
	}
	defer s.guard.Resume()
	if err := s.save(record{Controller: s.core.Controller()}); err != nil {
		return err
	}
	s.startupErr = ""
	return s.core.Stop()
}

func (s *supervisor) retryRestore(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		s.mu.Lock()
		if s.restorePending != nil {
			if err := s.start(*s.restorePending, true); err == nil {
				s.restorePending = nil
			} else {
				s.startupErr = err.Error()
			}
		}
		s.mu.Unlock()
	}
}

func (s *supervisor) Test() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.core.Test(); err != nil {
		return fmt.Errorf("core configuration: %w", err)
	}
	return nil
}
