// Package appdir names where ClashCube keeps its files.
package appdir

import (
	"os"
	"path/filepath"
	"runtime"
)

// Root is the platform's per-user ClashCube data directory, or CLASHCUBE_HOME.
func Root() string {
	if d := os.Getenv("CLASHCUBE_HOME"); d != "" {
		return d
	}
	if runtime.GOOS == "linux" {
		d, err := os.UserConfigDir()
		if err == nil {
			return filepath.Join(d, "clashcube")
		}
		return filepath.Join(os.TempDir(), "clashcube")
	}
	if runtime.GOOS == "windows" {
		d, err := os.UserConfigDir()
		if err == nil {
			return filepath.Join(d, "ClashCube")
		}
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "AppData", "Roaming", "ClashCube")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Application Support", "ClashCube")
}

func Settings() string { return filepath.Join(Root(), "settings.json") }
func Profiles() string { return filepath.Join(Root(), "profiles") }

// CoreHome may be volatile or external on a router. Runtime configuration and
// app preferences remain in Root so a reboot can restore the validated state.
func CoreHome() string {
	if d := os.Getenv("CLASHCUBE_CORE_HOME"); d != "" {
		return d
	}
	return filepath.Join(Root(), "core")
}
func RuntimeConfig() string { return filepath.Join(Root(), "core", "runtime.yaml") }
func CacheHome() string {
	if d := os.Getenv("CLASHCUBE_CACHE_HOME"); d != "" {
		return d
	}
	return Root()
}
func Logs() string  { return filepath.Join(CacheHome(), "logs") }
func Usage() string { return filepath.Join(CacheHome(), "usage") }

// Ensure creates the directories the app writes to.
func Ensure() error {
	for _, d := range []string{Root(), Profiles(), CoreHome(), Logs()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}
