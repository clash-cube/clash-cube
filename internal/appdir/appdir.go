// Package appdir names where MihomoBar keeps its files.
package appdir

import (
	"os"
	"path/filepath"
)

// Root is ~/Library/Application Support/MihomoBar, or $MIHOMOBAR_HOME.
func Root() string {
	if d := os.Getenv("MIHOMOBAR_HOME"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Application Support", "MihomoBar")
}

func Settings() string      { return filepath.Join(Root(), "settings.json") }
func Profiles() string      { return filepath.Join(Root(), "profiles") }
func CoreHome() string      { return filepath.Join(Root(), "core") }
func RuntimeConfig() string { return filepath.Join(CoreHome(), "runtime.yaml") }
func Logs() string          { return filepath.Join(Root(), "logs") }
func Usage() string         { return filepath.Join(Root(), "usage") }

// Ensure creates the directories the app writes to.
func Ensure() error {
	for _, d := range []string{Root(), Profiles(), CoreHome(), Logs()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}
