package hotkeys

import "strings"

func init() {
	for action, keys := range Recommended {
		Recommended[action] = strings.ReplaceAll(keys, "Cmd+", "Shift+")
	}
}

// RegisterHotKey, used by Wails, reports reserved and occupied combinations.
func systemTaken(string) bool { return false }
