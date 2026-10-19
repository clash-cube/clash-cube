package gui

// Hotkey is an action's global shortcut, as Settings shows it.
type Hotkey struct {
	Action string `json:"action"`
	Keys   string `json:"keys"`            // as "Ctrl+Option+Cmd+P"; "" for none
	Error  string `json:"error,omitempty"` // why it isn't working
}

// Recommended is what UseRecommendedHotkeys did: the shortcuts now, and
// the actions it left without one, the shortcut being taken.
type Recommended struct {
	Hotkeys []Hotkey `json:"hotkeys"`
	Skipped []string `json:"skipped"`
}
