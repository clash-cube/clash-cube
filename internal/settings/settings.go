// Package settings is the app's own settings, kept in settings.json.
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/localhost-copilot/mihomobar/internal/appdir"
)

type Settings struct {
	// Profile is the id of the profile in use.
	Profile string `json:"profile"`

	// What the app lays over every profile (runtimecfg).
	Mode      string `json:"mode"` // rule | global | direct
	MixedPort int    `json:"mixedPort"`
	AllowLan  bool   `json:"allowLan"`
	IPv6      bool   `json:"ipv6"`
	LogLevel  string `json:"logLevel"`
	TunStack  string `json:"tunStack"` // system | gvisor | mixed
	// ICMPForwarding sends pings under TUN out directly; off, the core
	// answers them itself. ICMP is never proxied either way.
	ICMPForwarding bool `json:"icmpForwarding"`

	SystemProxy bool     `json:"systemProxy"`
	Tun         bool     `json:"tun"`
	Bypass      []string `json:"bypass"`

	// The core runs through the root helper (service mode) once TUN has been
	// turned on; it stays there until the helper is removed.
	ServiceMode bool `json:"serviceMode"`

	AutoStart     bool   `json:"autoStart"` // start the core when the app opens
	LaunchAtLogin bool   `json:"launchAtLogin"`
	Theme         string `json:"theme"` // system | light | dark
	Lang          string `json:"lang"`  // system | en | zh
	Dock          string `json:"dock"`  // never | always | window
	TestURL       string `json:"testUrl"`
	FindProcess   bool   `json:"findProcess"` // look up the process of every connection
	TraySpeed     bool   `json:"traySpeed"`   // speed beside the menu bar icon
	Window        []int  `json:"window,omitempty"`
}

func Defaults() Settings {
	return Settings{
		Mode:           "rule",
		MixedPort:      7890,
		LogLevel:       "info",
		TunStack:       "mixed",
		ICMPForwarding: true,
		Bypass: []string{
			"127.0.0.1", "192.168.0.0/16", "10.0.0.0/8", "172.16.0.0/12",
			"localhost", "*.local", "*.crashlytics.com", "<local>",
		},
		AutoStart:   true,
		FindProcess: true,
		Theme:       "system",
		Lang:        "system",
		Dock:        "window",
		TestURL:     "https://www.gstatic.com/generate_204",
	}
}

var (
	mu       sync.Mutex // the file
	updateMu sync.Mutex // read-modify-write cycles
)

// Load reads settings.json over the defaults; a missing or bad file is the
// defaults.
func Load() Settings {
	mu.Lock()
	defer mu.Unlock()
	s := Defaults()
	if b, err := os.ReadFile(appdir.Settings()); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	return s
}

// Save writes s atomically.
func Save(s Settings) error {
	mu.Lock()
	defer mu.Unlock()
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(appdir.Settings(), b, 0o644)
}

// Update loads, applies fn and saves, under one lock.
func Update(fn func(*Settings)) (Settings, error) {
	updateMu.Lock()
	defer updateMu.Unlock()
	s := Load()
	fn(&s)
	return s, Save(s)
}

func writeAtomic(path string, b []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
