// Package coremgr runs the core: `clashcube core` as a child of the GUI, on a
// controller address and secret made fresh at each start.
package coremgr

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/localhost-copilot/clashcube/internal/mihomoapi"
	"github.com/localhost-copilot/clashcube/internal/runtimecfg"
)

type Status string

// ErrServiceStopped distinguishes an intentional platform stop (notification
// action or VPN revocation) from an unexpected service-process death.
var ErrServiceStopped = errors.New("core service stopped")

const (
	Stopped  Status = "stopped"
	Starting Status = "starting"
	Running  Status = "running"
	Stopping Status = "stopping"
	Crashed  Status = "crashed"
)

// Runner starts and stops the core process. The user-mode runner forks it
// here; the service-mode one asks the root helper to.
type Runner interface {
	// Start runs the core on home/config, its controller at ctl. exited
	// is closed when the process ends; output gets its lines.
	Start(home, config string, ctl runtimecfg.Controller, output func(string)) (exited <-chan error, err error)
	Stop() error
	// Test checks a configuration without running it.
	Test(home, config string) error
}

// Manager keeps one core running and knows how to reach it.
type Manager struct {
	runner Runner
	home   string
	config string

	mu       sync.Mutex
	status   Status
	ctl      runtimecfg.Controller
	client   *mihomoapi.Client
	lastErr  string
	tail     []string
	gen      int // starts; an exit of an older one is ignored
	onChange func()
	onLog    func(string)
}

func New(runner Runner, home, config string) *Manager {
	return &Manager{runner: runner, home: home, config: config, status: Stopped}
}

// OnChange is called after every status change, off the lock.
func (m *Manager) OnChange(fn func()) { m.onChange = fn }

// OnLog gets the core's own output lines.
func (m *Manager) OnLog(fn func(string)) { m.onLog = fn }

// SetRunner switches between user and service mode; takes effect at the
// next start.
func (m *Manager) SetRunner(r Runner) {
	m.mu.Lock()
	m.runner = r
	m.mu.Unlock()
}

// Runner is the runner the core runs (or will run) under.
func (m *Manager) Runner() Runner {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runner
}

func (m *Manager) Status() (Status, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status, m.lastErr
}

// Client is the API of the running core, nil when it isn't.
func (m *Manager) Client() *mihomoapi.Client {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status != Running {
		return nil
	}
	return m.client
}

// Controller is where the next (or current) core listens.
func (m *Manager) Controller() runtimecfg.Controller {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ctl
}

// Tail is the last lines the core wrote.
func (m *Manager) Tail() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.tail...)
}

func (m *Manager) set(s Status, errText string) {
	m.mu.Lock()
	m.status = s
	m.lastErr = errText
	m.mu.Unlock()
	if m.onChange != nil {
		m.onChange()
	}
}

// NewController picks a free port and a random secret, for the runtime
// configuration about to be written.
func (m *Manager) NewController() (runtimecfg.Controller, error) {
	port, err := freePort()
	if err != nil {
		return runtimecfg.Controller{}, err
	}
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	ctl := runtimecfg.Controller{Addr: fmt.Sprintf("127.0.0.1:%d", port), Secret: hex.EncodeToString(b)}
	m.mu.Lock()
	m.ctl = ctl
	m.mu.Unlock()
	return ctl, nil
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// Test checks the runtime configuration.
func (m *Manager) Test() error {
	m.mu.Lock()
	r := m.runner
	m.mu.Unlock()
	return r.Test(m.home, m.config)
}

// Start runs the core on the runtime configuration (written for the
// controller NewController gave) and waits until its API answers.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.status == Running || m.status == Starting {
		m.mu.Unlock()
		return nil
	}
	m.gen++
	gen := m.gen
	ctl, r := m.ctl, m.runner
	m.tail = nil
	m.mu.Unlock()
	if ctl.Addr == "" {
		return errors.New("no controller: call NewController and write the configuration first")
	}

	m.set(Starting, "")
	exited, err := r.Start(m.home, m.config, ctl, m.addLine)
	if err != nil {
		m.set(Crashed, err.Error())
		return err
	}
	client := mihomoapi.New(ctl.Addr, ctl.Secret)

	ready := make(chan error, 1)
	go func() {
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			c, cancel := context.WithTimeout(ctx, time.Second)
			_, err := client.Version(c)
			cancel()
			if err == nil {
				ready <- nil
				return
			}
			select {
			case <-ctx.Done():
				ready <- ctx.Err()
				return
			case <-time.After(150 * time.Millisecond):
			}
		}
		ready <- errors.New("core did not answer within 15s")
	}()

	select {
	case err := <-ready:
		if err != nil {
			_ = r.Stop()
			m.set(Crashed, m.failure(err))
			return err
		}
	case err := <-exited:
		m.set(Crashed, m.failure(err))
		return errors.New(m.failure(err))
	}

	m.mu.Lock()
	m.client = client
	m.mu.Unlock()
	m.set(Running, "")

	go func() {
		err := <-exited
		m.mu.Lock()
		current := gen == m.gen && m.status == Running
		m.mu.Unlock()
		if current {
			m.recordExit(err)
		}
	}()
	return nil
}

func (m *Manager) recordExit(err error) {
	if errors.Is(err, ErrServiceStopped) {
		m.set(Stopped, "")
		return
	}
	m.set(Crashed, m.failure(err))
}

// failure is err with the core's last error line, which says more.
func (m *Manager) failure(err error) string {
	tail := m.Tail()
	for i := len(tail) - 1; i >= 0; i-- {
		if l := tail[i]; strings.Contains(l, "level=fatal") || strings.Contains(l, "level=error") || strings.Contains(l, "failed") {
			if j := strings.Index(l, "msg="); j >= 0 {
				return strings.Trim(l[j+4:], `"`)
			}
			return l
		}
	}
	if err == nil {
		return "core exited"
	}
	return err.Error()
}

func (m *Manager) addLine(l string) {
	m.mu.Lock()
	m.tail = append(m.tail, l)
	if len(m.tail) > 50 {
		m.tail = m.tail[len(m.tail)-50:]
	}
	m.mu.Unlock()
	if m.onLog != nil {
		m.onLog(l)
	}
}

// Stop ends the core.
func (m *Manager) Stop() error {
	m.mu.Lock()
	if m.status == Stopped {
		m.mu.Unlock()
		return nil
	}
	m.gen++
	r := m.runner
	m.client = nil
	m.mu.Unlock()
	m.set(Stopping, "")
	err := r.Stop()
	m.set(Stopped, "")
	return err
}
