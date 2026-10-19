package coremgr

import (
	"errors"
	"sync"
	"time"

	"github.com/localhost-copilot/clashcube/internal/androidbridge"
	"github.com/localhost-copilot/clashcube/internal/runtimecfg"
	"github.com/localhost-copilot/clashcube/internal/settings"
)

var ErrAndroidCore = errors.New("Android uses the VPN service; desktop helper controls are unavailable")

type AndroidRunner struct {
	mu   sync.Mutex
	done chan error
}

func (r *AndroidRunner) watch() <-chan error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.done = make(chan error, 1)
	done := r.done
	// Native status checks Binder liveness, not transient REST availability.
	go func() {
		for {
			select {
			case <-time.After(time.Second):
			case <-androidbridge.ServiceEvents:
			}
			var state struct {
				Running bool `json:"running"`
				Stopped bool `json:"stopped"`
			}
			err := androidbridge.Call(map[string]any{"op": "status"}, &state)
			if err != nil || !state.Running {
				if err == nil && state.Stopped {
					done <- ErrServiceStopped
					return
				}
				done <- errors.New("Android core service stopped or died")
				return
			}
			r.mu.Lock()
			current := r.done == done
			r.mu.Unlock()
			if !current {
				done <- ErrServiceStopped
				return
			}
		}
	}()
	return done
}
func (r *AndroidRunner) Start(home, file string, ctl runtimecfg.Controller, output func(string)) (<-chan error, error) {
	err := androidbridge.Call(map[string]any{"op": "start", "home": home, "file": file, "addr": ctl.Addr, "secret": ctl.Secret, "ipv6": settings.Load().IPv6}, nil)
	if err != nil {
		return nil, err
	}
	return r.watch(), nil
}
func (*AndroidRunner) Test(home, file string) error {
	return androidbridge.Call(map[string]any{"op": "check", "home": home, "file": file}, nil)
}
func (*AndroidRunner) Stop() error { return androidbridge.Call(map[string]any{"op": "stop"}, nil) }
func (r *AndroidRunner) Attach() (runtimecfg.Controller, <-chan error, error) {
	var state struct {
		Running bool   `json:"running"`
		Addr    string `json:"addr"`
		Secret  string `json:"secret"`
	}
	if err := androidbridge.Call(map[string]any{"op": "attach"}, &state); err != nil {
		return runtimecfg.Controller{}, nil, err
	}
	if !state.Running {
		return runtimecfg.Controller{}, nil, nil
	}
	return runtimecfg.Controller{Addr: state.Addr, Secret: state.Secret}, r.watch(), nil
}
