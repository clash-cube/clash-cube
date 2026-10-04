package gui

import (
	"log"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/localhost-copilot/mihomobar/internal/settings"
)

// shortcuts keeps the global shortcuts registered as the settings say.
type shortcuts struct {
	h   *host
	mu  sync.Mutex
	set map[string]string // action → the shortcut registered for it
	// paused while the page records one, so pressing a shortcut that is
	// already set records it rather than running its action
	paused bool
}

// apply registers what the settings ask for and drops the rest. An action
// whose shortcut the system won't give (another app holds it) is left
// without one; the result says why, by action.
func (s *shortcuts) apply() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	want := map[string]string{}
	if !s.paused {
		for a, k := range settings.Load().Hotkeys {
			if k != "" {
				want[a] = k
			}
		}
	}
	gs := s.h.app.GlobalShortcut
	for a, k := range s.set {
		if want[a] != k {
			_ = gs.Unregister(k)
			delete(s.set, a)
		}
	}
	failed := map[string]string{}
	for a, k := range want {
		if s.set[a] == k {
			continue
		}
		action := a
		if err := gs.Register(k, func() { s.run(action) }); err != nil {
			log.Printf("shortcut %s for %s: %v", k, a, err)
			failed[a] = err.Error()
			continue
		}
		s.set[a] = k
	}
	return failed
}

// pause lifts every shortcut while one is recorded, and puts them back.
func (s *shortcuts) pause(on bool) map[string]string {
	s.mu.Lock()
	s.paused = on
	s.mu.Unlock()
	return s.apply()
}

func (s *shortcuts) run(action string) {
	h := s.h
	st := h.b.State()
	switch action {
	case "panel":
		application.InvokeAsync(h.tray.ToggleWindow)
	case "main":
		application.InvokeAsync(func() {
			if h.main.IsVisible() && h.main.IsFocused() {
				h.hideMain()
				return
			}
			h.showMain("")
		})
	case "mode":
		// Rule → Global → Direct, as the tray menu lists them
		next := map[string]string{"rule": "global", "global": "direct", "direct": "rule"}[st.Mode]
		if next == "" {
			next = "rule"
		}
		s.report(h.b.SetMode(next), tr("Outbound Mode", "出站模式"))
	case "systemProxy":
		s.report(h.b.SetSystemProxy(!st.SystemProxy || st.ProxyLost), tr("System Proxy", "系统代理"))
	case "tun":
		s.report(h.b.SetTun(!st.Tun, helperPrompt()), tr("Enhanced Mode", "增强模式"))
	}
}

// report answers a shortcut pressed in another app, where nothing of ours
// may be on screen. The tray icon already shows the new mode or state;
// lighting it says the press was taken. A failure is posted.
func (s *shortcuts) report(err error, title string) {
	if err != nil {
		scriptNotify(title, err.Error())
		return
	}
	application.InvokeAsync(trayPlay)
}
