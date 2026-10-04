package hotkeys

import (
	"errors"
	"testing"
)

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"cmd+ctrl+k":         "Ctrl+Cmd+K",
		"Shift+Option+Cmd+M": "Option+Shift+Cmd+M",
		"ctrl+option+space":  "Ctrl+Option+Space",
		"f18":                "F18",
		"ctrl+up":            "Ctrl+Up",
	} {
		if got, err := Normalize(in); err != nil || got != want {
			t.Errorf("%s = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"k", "shift+k", "option+k", "option+shift+k", "cmd+k", "ctrl+escape", "ctrl+§", "hyper+k"} {
		if got, err := Normalize(in); err == nil {
			t.Errorf("%s accepted as %q", in, got)
		}
	}
}

func TestCheck(t *testing.T) {
	set := map[string]string{"panel": "Ctrl+Option+Cmd+P", "main": ""}
	var p Problem
	if _, err := Check("tun", "cmd+option+ctrl+p", set); !errors.As(err, &p) || p.Action != "panel" {
		t.Errorf("a shortcut already in use: %v", err)
	}
	// the action's own shortcut again is fine
	if got, err := Check("panel", "ctrl+option+cmd+p", set); err != nil || got != "Ctrl+Option+Cmd+P" {
		t.Errorf("its own shortcut: %q, %v", got, err)
	}
	if _, err := Check("nope", "ctrl+option+cmd+p", set); err == nil {
		t.Error("an unknown action accepted")
	}
	// the screenshot shortcut, on by default
	if _, err := Check("panel", "shift+cmd+3", nil); !errors.As(err, &p) || p.Text != "macOS uses this shortcut" {
		t.Errorf("⇧⌘3: %v", err)
	}
}

// Every recommended shortcut is one Check takes, and they are distinct.
func TestRecommended(t *testing.T) {
	seen := map[string]bool{}
	for _, a := range Actions {
		k, ok := Recommended[a]
		if !ok {
			t.Fatalf("no recommended shortcut for %s", a)
		}
		norm, err := Check(a, k, nil)
		if err != nil || norm != k || seen[k] {
			t.Errorf("%s: %q, %v", a, norm, err)
		}
		seen[k] = true
	}
}
