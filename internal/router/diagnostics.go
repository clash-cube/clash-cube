package router

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/localhost-copilot/clashcube/internal/appdir"
)

type StorageInfo struct {
	Path  string `json:"path"`
	Total uint64 `json:"total"`
	Free  uint64 `json:"free"`
}

type Check struct {
	Name string `json:"name"`
	OK   bool   `json:"ok"`
}

type PackageInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Diagnostics is deliberately a projection, not a configuration/log dump:
// subscription URLs, controller secrets, DHCP identities and node credentials
// never enter an exported diagnostic report.
type Diagnostics struct {
	System      string        `json:"system"`
	Kernel      string        `json:"kernel"`
	Uptime      int64         `json:"uptime"`
	MemoryTotal uint64        `json:"memoryTotal"`
	MemoryFree  uint64        `json:"memoryFree"`
	AppVersion  string        `json:"appVersion"`
	CoreVersion string        `json:"coreVersion"`
	Core        string        `json:"core"`
	Storage     []StorageInfo `json:"storage"`
	Packages    []PackageInfo `json:"packages"`
	Checks      []Check       `json:"checks"`
}

func collectDiagnostics(exec command, home, coreHome, cacheHome string) Diagnostics {
	d := Diagnostics{Storage: []StorageInfo{}, Packages: []PackageInfo{}, Checks: []Check{}}
	var board struct {
		Kernel  string `json:"kernel"`
		Release struct {
			Description string `json:"description"`
		} `json:"release"`
	}
	if body, err := exec("ubus", "call", "system", "board"); err == nil && json.Unmarshal(body, &board) == nil {
		d.System, d.Kernel = board.Release.Description, board.Kernel
	}
	var info struct {
		Uptime int64                             `json:"uptime"`
		Memory struct{ Total, Available uint64 } `json:"memory"`
	}
	if body, err := exec("ubus", "call", "system", "info"); err == nil && json.Unmarshal(body, &info) == nil {
		d.Uptime, d.MemoryTotal, d.MemoryFree = info.Uptime, info.Memory.Total, info.Memory.Available
	}
	for _, path := range []string{home, coreHome, cacheHome} {
		total, free, err := storageBytes(path)
		d.Storage = append(d.Storage, StorageInfo{Path: path, Total: total, Free: free})
		d.Checks = append(d.Checks, Check{Name: path, OK: err == nil})
	}
	if body, err := exec("opkg", "list-installed", "*clashcube*"); err == nil {
		for _, line := range strings.Split(string(body), "\n") {
			name, version, ok := strings.Cut(line, " - ")
			if ok && (name == "clashcube-core" || name == "clashcube-web" || name == "luci-app-clashcube") {
				d.Packages = append(d.Packages, PackageInfo{Name: name, Version: version})
			}
		}
	}
	version, err := exec("iptables", "--version")
	d.Checks = append(d.Checks, Check{Name: "fw3 / iptables-legacy", OK: err == nil && strings.Contains(string(version), "legacy")})
	_, err = exec("/etc/init.d/dnsmasq", "running")
	d.Checks = append(d.Checks, Check{Name: "dnsmasq", OK: err == nil})
	return d
}

func (m *Manager) Diagnostics() Diagnostics {
	d := collectDiagnostics(run, filepath.Dir(m.path), appdir.CoreHome(), appdir.CacheHome())
	s := m.b.State()
	d.AppVersion, d.CoreVersion, d.Core = s.AppVersion, s.CoreVersion, s.Core
	return d
}
