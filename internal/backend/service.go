package backend

import (
	"log"
	"strconv"

	"github.com/localhost-copilot/mihomobar/internal/appdir"
	"github.com/localhost-copilot/mihomobar/internal/coremgr"
	"github.com/localhost-copilot/mihomobar/internal/helper"
	"github.com/localhost-copilot/mihomobar/internal/settings"
)

// The runners the core can run under.
var (
	localRunner   = &coremgr.LocalRunner{}
	helperInstall = helper.Install
)

func serviceRunner() coremgr.Runner {
	return &helper.Runner{Test_: localRunner.Test}
}

// pickRunner sets the runner the settings ask for: through the helper in
// service mode if it answers, else as a child. Called before a start.
func (b *Backend) pickRunner() {
	if settings.Load().ServiceMode {
		if running, _ := helperInstalled(); running {
			b.core.SetRunner(serviceRunner())
			return
		}
		log.Println("service mode is on but the helper does not answer; running as a child")
	}
	b.core.SetRunner(localRunner)
}

// HelperStatus is what Settings shows of service mode.
type HelperStatus struct {
	Installed bool `json:"installed"`
	Current   bool `json:"current"` // runs this version of the app
	Enabled   bool `json:"enabled"` // settings.ServiceMode
	Active    bool `json:"active"`  // the running core is the helper's
}

func (b *Backend) HelperStatus() HelperStatus {
	running, current := helper.Installed()
	_, active := b.core.Runner().(*helper.Runner)
	return HelperStatus{Installed: running, Current: current, Enabled: settings.Load().ServiceMode, Active: active && b.core.Client() != nil}
}

// EnableServiceMode installs (or updates) the helper, asking for an
// administrator's password, and moves the core to it.
func (b *Backend) EnableServiceMode(prompt string) error {
	// Replacing the helper disconnects its core. Remember the running state
	// before installation, while the old helper is still alive.
	wasRunning := b.core.Client() != nil
	if running, current := helperInstalled(); !running || !current {
		if err := helperInstall(appdir.Root(), prompt); err != nil {
			return err
		}
	}
	if _, err := settings.Update(func(s *settings.Settings) { s.ServiceMode = true }); err != nil {
		return err
	}
	b.emitState()
	if wasRunning {
		return b.Restart()
	}
	return nil
}

// DisableServiceMode moves the core back to running as the user, turning
// TUN off; uninstall also removes the helper.
func (b *Backend) DisableServiceMode(uninstall bool, prompt string) error {
	if _, err := settings.Update(func(s *settings.Settings) { s.ServiceMode = false; s.Tun = false }); err != nil {
		return err
	}
	wasRunning := b.core.Client() != nil
	if wasRunning {
		if err := b.Stop(); err != nil {
			return err
		}
	}
	if uninstall {
		if err := helper.Uninstall(prompt); err != nil {
			return err
		}
	}
	b.emitState()
	if wasRunning {
		return b.Start()
	}
	return nil
}

// SetTun turns TUN on or off. TUN needs the core to run as root, so turning
// it on first moves the core to the helper (installing it if need be).
func (b *Backend) SetTun(on bool, prompt string) error {
	if err := b.setTun(on, prompt); err != nil {
		return err
	}
	b.noteManual("tun", strconv.FormatBool(on))
	return nil
}

func (b *Backend) setTun(on bool, prompt string) error {
	if on {
		if _, active := b.core.Runner().(*helper.Runner); !active || !settings.Load().ServiceMode {
			if _, err := settings.Update(func(s *settings.Settings) { s.Tun = true }); err != nil {
				return err
			}
			if err := b.EnableServiceMode(prompt); err != nil {
				_, _ = settings.Update(func(s *settings.Settings) { s.Tun = false })
				b.emitState()
				return err
			}
			if b.core.Client() == nil {
				return b.Start()
			}
			return nil
		}
	}
	if _, err := settings.Update(func(s *settings.Settings) { s.Tun = on }); err != nil {
		return err
	}
	b.emitState()
	if b.core.Client() == nil {
		return nil
	}
	return b.Reload()
}
