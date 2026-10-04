package gui

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/localhost-copilot/clashcube/internal/backend"
	"github.com/localhost-copilot/clashcube/internal/mihomoapi"
	"github.com/localhost-copilot/clashcube/internal/profiles"
)

func init() {
	application.RegisterEvent[backend.State]("state")
	application.RegisterEvent[mihomoapi.Traffic]("traffic")
	application.RegisterEvent[mihomoapi.Memory]("memory")
	application.RegisterEvent[mihomoapi.Log]("log")
	application.RegisterEvent[[]profiles.Profile]("profiles")
	application.RegisterEvent[string]("navigate")
	application.RegisterEvent[bool]("import-request")
	application.RegisterEvent[backend.LatencyEvent]("proxy-latency")
	application.RegisterEvent[backend.Event]("event")
	application.RegisterEvent[backend.LatencySample]("connectivity")
}

// sink turns the backend's changes into events for both pages, and keeps
// the tray up to date.
type sink struct{ h *host }

func (s sink) emit(name string, data any) {
	if s.h.app != nil {
		s.h.app.Event.Emit(name, data)
	}
}

func (s sink) State(st backend.State) {
	s.emit("state", st)
	s.h.stateChanged(st)
}
func (s sink) Traffic(t mihomoapi.Traffic) {
	s.emit("traffic", t)
	s.h.trafficChanged(t)
}
func (s sink) Memory(m mihomoapi.Memory)       { s.emit("memory", m) }
func (s sink) Log(l mihomoapi.Log)             { s.emit("log", l) }
func (s sink) Profiles(ps []profiles.Profile)  { s.emit("profiles", ps) }
func (s sink) Latency(l backend.LatencySample) { s.emit("connectivity", l) }
func (s sink) Event(e backend.Event) {
	s.emit("event", e)
	if (e.Text == backend.ResetText || e.Text == backend.OfflineText) && s.h.menu != nil {
		s.h.menu.remeasure()
	}
	if e.Notify {
		go s.h.notify(e)
	}
}
