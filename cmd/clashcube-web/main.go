// clashcube-web runs replaceable management behind LuCI authentication.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/localhost-copilot/clashcube/internal/appdir"
	"github.com/localhost-copilot/clashcube/internal/backend"
	"github.com/localhost-copilot/clashcube/internal/coremgr"
	"github.com/localhost-copilot/clashcube/internal/coreservice"
	"github.com/localhost-copilot/clashcube/internal/defaults"
	"github.com/localhost-copilot/clashcube/internal/profiles"
	"github.com/localhost-copilot/clashcube/internal/router"
	"github.com/localhost-copilot/clashcube/internal/routerupdate"
	"github.com/localhost-copilot/clashcube/internal/settings"
	"github.com/localhost-copilot/clashcube/internal/webui"
)

var version = "dev"

func main() {
	if len(os.Args) == 2 && os.Args[1] == "update-core" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if err := routerupdate.New(filepath.Join(appdir.CacheHome(), "updates")).Run(ctx); err != nil {
			log.Fatal(err)
		}
		return
	}
	socket := flag.String("socket", "/var/run/clashcube/web.sock", "private LuCI bridge socket")
	flag.Parse()
	if err := run(*socket); err != nil {
		log.Fatal(err)
	}
}

func run(socket string) error {
	l, err := webui.Listen(socket)
	if err != nil {
		return err
	}
	defer l.Close()
	if err := appdir.Ensure(); err != nil {
		return err
	}
	if _, err := os.Stat(appdir.Settings()); os.IsNotExist(err) {
		s := settings.Defaults()
		s.MixedPort, s.AllowLan, s.FindProcess = 7897, true, false
		s.AIServices = []string{}
		if err := settings.Save(s); err != nil {
			return err
		}
	}
	// Desktop privilege and network automation do not apply to router services.
	if _, err := settings.Update(func(s *settings.Settings) {
		s.SystemProxy, s.Tun, s.ServiceMode, s.NetworkAuto = false, false, false, false
		s.LaunchAtLogin, s.AutoUpdateApp, s.Notify, s.SaveData = false, false, false, false
	}); err != nil {
		return err
	}
	service := coreservice.NewClient()
	defer service.Close()
	status, err := service.Status()
	if err != nil {
		return err
	}
	events := webui.NewBroker()
	b := backend.New(version, status.Version, defaults.Profile, events)
	b.CoreService = service
	r, err := router.New(b, appdir.Root(), service.Firewall())
	if err != nil {
		return err
	}
	if err := b.Init(); err != nil {
		return err
	}
	defer b.Shutdown()
	if status.Status == coremgr.Running {
		if err := b.AttachCore(status.Controller, service.Observe(status.Controller)); err != nil {
			return err
		}
	} else if !status.Initialized && settings.Load().AutoStart {
		if err := b.Start(); err != nil {
			log.Printf("start core: %v", err)
		}
	}
	events.Profiles(profiles.List())
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	go reconnect(ctx, b, service)
	// Release a transaction left by a crashed previous management session.
	// This reconciles existing rules without rebuilding an unchanged chain.
	if _, err := service.Firewall().Resume(); err != nil {
		log.Printf("router status: %v", err)
	}
	server := &http.Server{Handler: webui.NewHandler(b, events, r), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second}
	b.Boot()
	stopped := make(chan error, 1)
	go func() { stopped <- server.Serve(l) }()
	select {
	case err := <-stopped:
		if err != http.ErrServerClosed {
			return err
		}
	case <-ctx.Done():
	}
	shutdown, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(shutdown); err != nil {
		return server.Close()
	}
	return nil
}

func reconnect(ctx context.Context, b *backend.Backend, service *coreservice.Client) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		status, err := service.Status()
		if err != nil {
			continue
		}
		b.ObserveCoreVersion(status.Version)
		if status.Status == coremgr.Stopped {
			b.ObserveCoreStopped()
		}
		if b.State().Core != "running" && status.Status == coremgr.Running {
			if err := b.AttachCore(status.Controller, service.Observe(status.Controller)); err != nil {
				log.Printf("reconnect core: %v", err)
			}
		}
	}
}
