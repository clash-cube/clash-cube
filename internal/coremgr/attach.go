package coremgr

import (
	"context"

	"github.com/localhost-copilot/clashcube/internal/mihomoapi"
	"github.com/localhost-copilot/clashcube/internal/runtimecfg"
)

// Attach adopts an independently owned service after UI process recreation.
// Credentials come from private platform IPC, never an unauthenticated API.
func (m *Manager) Attach(ctx context.Context) (bool, error) {
	r, ok := m.Runner().(interface {
		Attach() (runtimecfg.Controller, <-chan error, error)
	})
	if !ok {
		return false, nil
	}
	ctl, exited, err := r.Attach()
	if err != nil || ctl.Addr == "" {
		return false, err
	}
	client := mihomoapi.New(ctl.Addr, ctl.Secret)
	if _, err := client.Version(ctx); err != nil {
		return false, err
	}
	m.mu.Lock()
	m.gen++
	gen := m.gen
	m.ctl = ctl
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
	return true, nil
}
