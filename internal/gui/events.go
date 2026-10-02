package gui

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/localhost-copilot/mihomobar/internal/backend"
	"github.com/localhost-copilot/mihomobar/internal/mihomoapi"
	"github.com/localhost-copilot/mihomobar/internal/profiles"
)

func init() {
	application.RegisterEvent[backend.State]("state")
	application.RegisterEvent[mihomoapi.Traffic]("traffic")
	application.RegisterEvent[mihomoapi.Memory]("memory")
	application.RegisterEvent[mihomoapi.Log]("log")
	application.RegisterEvent[[]profiles.Profile]("profiles")
	application.RegisterEvent[string]("navigate")
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
func (s sink) Memory(m mihomoapi.Memory)      { s.emit("memory", m) }
func (s sink) Log(l mihomoapi.Log)            { s.emit("log", l) }
func (s sink) Profiles(ps []profiles.Profile) { s.emit("profiles", ps) }
