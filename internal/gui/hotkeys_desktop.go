//go:build !android

package gui

import (
	"errors"
	"slices"

	"github.com/localhost-copilot/clashcube/internal/hotkeys"
	"github.com/localhost-copilot/clashcube/internal/settings"
)

// Hotkeys is every action and its shortcut.
func (s *SettingsService) Hotkeys() []Hotkey {
	return s.h.hotkeys(s.h.keys.apply())
}

func (h *host) hotkeys(failed map[string]string) []Hotkey {
	set := settings.Load().Hotkeys
	out := make([]Hotkey, 0, len(hotkeys.Actions))
	for _, a := range hotkeys.Actions {
		hk := Hotkey{Action: a, Keys: set[a]}
		if failed[a] != "" {
			hk.Error = "Another app is using this shortcut"
		}
		out = append(out, hk)
	}
	return out
}

// SetHotkey gives an action a shortcut ("" clears it). One refused (taken
// by macOS or another action, or not a usable combination) leaves the
// settings as they were; the error is a hotkeys.Problem.
func (s *SettingsService) SetHotkey(action, keys string) ([]Hotkey, error) {
	norm := ""
	if keys != "" {
		var err error
		if norm, err = hotkeys.Check(action, keys, settings.Load().Hotkeys); err != nil {
			return s.h.hotkeys(nil), err
		}
	} else if !slices.Contains(hotkeys.Actions, action) {
		return s.h.hotkeys(nil), errors.New("unknown action " + action)
	}
	if _, err := settings.Update(func(st *settings.Settings) {
		if st.Hotkeys == nil {
			st.Hotkeys = map[string]string{}
		}
		if norm == "" {
			delete(st.Hotkeys, action)
		} else {
			st.Hotkeys[action] = norm
		}
	}); err != nil {
		return s.h.hotkeys(nil), err
	}
	failed := s.h.keys.apply()
	if failed[action] != "" {
		// another app holds it: don't keep a shortcut that does nothing
		_, _ = settings.Update(func(st *settings.Settings) { delete(st.Hotkeys, action) })
		return s.h.hotkeys(nil), hotkeys.Problem{Text: "Another app is using this shortcut"}
	}
	return s.h.hotkeys(failed), nil
}

// RecordHotkey lifts every shortcut while the page records one, so that
// pressing a set one is recorded rather than run; false puts them back.
func (s *SettingsService) RecordHotkey(on bool) { s.h.keys.pause(on) }

// UseRecommendedHotkeys gives every action without a shortcut the
// recommended one; those already set are kept, and one macOS, another
// action or another app holds is skipped.
func (s *SettingsService) UseRecommendedHotkeys() (Recommended, error) {
	out := Recommended{Skipped: []string{}}
	for _, a := range hotkeys.Actions {
		if settings.Load().Hotkeys[a] != "" {
			continue
		}
		if _, err := s.SetHotkey(a, hotkeys.Recommended[a]); err != nil {
			out.Skipped = append(out.Skipped, a)
		}
	}
	out.Hotkeys = s.h.hotkeys(s.h.keys.apply())
	return out, nil
}
