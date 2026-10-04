// Package hotkeys is the app's global shortcuts: which actions take one,
// how a shortcut is written, and which ones are refused for getting in the
// way of the system or of every other app.
package hotkeys

import (
	"errors"
	"slices"
	"strings"
)

// Actions is what a shortcut can do, in the order Settings lists them.
var Actions = []string{"panel", "main", "mode", "systemProxy", "tun"}

// Recommended is the set "Use recommended shortcuts" fills in: ⌃⌥⌘ and a
// letter, which few apps take, the letters matching the tray menu's own
// (⌘M, ⌘S, ⌘E) where it has them. None is set until the user asks.
var Recommended = map[string]string{
	"panel":       "Ctrl+Option+Cmd+P",
	"main":        "Ctrl+Option+Cmd+M",
	"mode":        "Ctrl+Option+Cmd+O",
	"systemProxy": "Ctrl+Option+Cmd+S",
	"tun":         "Ctrl+Option+Cmd+E",
}

// Problem is why a shortcut is refused, in English for the page to
// translate; Action names the action that already has it.
type Problem struct {
	Text   string `json:"text"`
	Action string `json:"action,omitempty"`
}

func (p Problem) Error() string { return p.Text }

// The modifiers, in the order macOS writes them: ⌃⌥⇧⌘.
var modOrder = []string{"Ctrl", "Option", "Shift", "Cmd"}

var modNames = map[string]string{
	"ctrl": "Ctrl", "control": "Ctrl", "option": "Option", "alt": "Option",
	"shift": "Shift", "cmd": "Cmd", "command": "Cmd",
}

// Normalize writes a shortcut ("cmd+ctrl+k") as the app keeps it
// ("Ctrl+Cmd+K"), refusing one with a key it can't bind, or one that would
// take keys the user types or every app's own shortcuts.
func Normalize(accel string) (string, error) {
	parts := strings.Split(strings.TrimSpace(accel), "+")
	key := strings.ToLower(strings.TrimSpace(parts[len(parts)-1]))
	if _, ok := keyCodes[key]; !ok {
		return "", Problem{Text: "This key can't be a shortcut"}
	}
	have := map[string]bool{}
	for _, m := range parts[:len(parts)-1] {
		name, ok := modNames[strings.ToLower(strings.TrimSpace(m))]
		if !ok {
			return "", errors.New("unknown modifier " + m)
		}
		have[name] = true
	}
	fn := strings.HasPrefix(key, "f") && len(key) > 1
	switch {
	case len(have) == 0 && !fn:
		return "", Problem{Text: "Add ⌃, ⌥ or ⌘: a key alone is for typing"}
	case !have["Ctrl"] && !have["Cmd"] && have["Option"] && !fn:
		return "", Problem{Text: "⌥ alone types characters: add ⌃ or ⌘"}
	case !have["Ctrl"] && !have["Cmd"] && have["Shift"] && !fn:
		return "", Problem{Text: "Add ⌃, ⌥ or ⌘: a key alone is for typing"}
	case have["Cmd"] && len(have) == 1 && !fn:
		return "", Problem{Text: "Apps use ⌘ and a key themselves: add ⌃, ⌥ or ⇧"}
	}
	var out []string
	for _, m := range modOrder {
		if have[m] {
			out = append(out, m)
		}
	}
	return strings.Join(append(out, keyName(key)), "+"), nil
}

func keyName(key string) string {
	if len(key) == 1 {
		return strings.ToUpper(key)
	}
	if key[0] == 'f' {
		return strings.ToUpper(key)
	}
	return strings.ToUpper(key[:1]) + key[1:]
}

// Check is why accel can't be action's shortcut, beside the others set
// (action → shortcut); nil when it can. The shortcut is normalized first.
func Check(action, accel string, set map[string]string) (string, error) {
	if !slices.Contains(Actions, action) {
		return "", errors.New("unknown action " + action)
	}
	norm, err := Normalize(accel)
	if err != nil {
		return "", err
	}
	for a, other := range set {
		if a != action && other == norm {
			return "", Problem{Text: "Already the shortcut for {action}", Action: a}
		}
	}
	if systemTaken(norm) {
		return "", Problem{Text: "macOS uses this shortcut"}
	}
	return norm, nil
}

// carbon is the shortcut as Carbon's hot key API takes it: the key's code
// and the modifier mask.
func carbon(norm string) (code, mods uint32) {
	parts := strings.Split(norm, "+")
	code = uint32(keyCodes[strings.ToLower(parts[len(parts)-1])])
	for _, m := range parts[:len(parts)-1] {
		mods |= carbonMods[m]
	}
	return code, mods
}

var carbonMods = map[string]uint32{"Cmd": 0x0100, "Shift": 0x0200, "Option": 0x0800, "Ctrl": 0x1000}

// keyCodes is the keys a shortcut may end with, by the name Wails gives
// them, and their virtual key codes (HIToolbox/Events.h). Wails binds the
// same table; a key missing from it would fail to register.
var keyCodes = map[string]int{
	"a": 0, "s": 1, "d": 2, "f": 3, "h": 4, "g": 5, "z": 6, "x": 7,
	"c": 8, "v": 9, "b": 11, "q": 12, "w": 13, "e": 14, "r": 15, "y": 16,
	"t": 17, "o": 31, "u": 32, "i": 34, "p": 35, "l": 37, "j": 38, "k": 40,
	"n": 45, "m": 46,
	"1": 18, "2": 19, "3": 20, "4": 21, "6": 22, "5": 23, "9": 25, "7": 26,
	"8": 28, "0": 29,
	"=": 24, "-": 27, "]": 30, "[": 33, "'": 39, ";": 41, "\\": 42,
	",": 43, "/": 44, ".": 47, "`": 50,
	"return": 36, "tab": 48, "space": 49, "backspace": 51, "delete": 117,
	"home": 115, "page up": 116, "end": 119, "page down": 121,
	"left": 123, "right": 124, "down": 125, "up": 126,
	"f1": 122, "f2": 120, "f3": 99, "f4": 118, "f5": 96, "f6": 97,
	"f7": 98, "f8": 100, "f9": 101, "f10": 109, "f11": 103, "f12": 111,
	"f13": 105, "f14": 107, "f15": 113, "f16": 106, "f17": 64, "f18": 79,
	"f19": 80, "f20": 90,
}
