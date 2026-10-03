// Package runtimecfg writes the configuration the core runs: the profile in
// use with the app's settings laid over it, so the core starts in the state
// the user asked for instead of being patched into it afterwards.
package runtimecfg

import (
	"fmt"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"

	"github.com/localhost-copilot/mihomobar/internal/settings"
	"github.com/localhost-copilot/mihomobar/internal/userrules"
)

// Controller is where the core's API listens and the secret it takes.
type Controller struct {
	Addr   string
	Secret string
}

// Build is profile with s and ctl laid over it, and the user's rules
// ahead of its own.
func Build(profile []byte, s settings.Settings, ctl Controller, user []userrules.Rule) ([]byte, error) {
	var m map[string]any
	if err := yaml.Unmarshal(profile, &m); err != nil {
		return nil, fmt.Errorf("profile: %w", err)
	}
	if m == nil {
		m = map[string]any{}
	}
	if s.MixedPort > 0 {
		m["mixed-port"] = s.MixedPort
	}
	m["allow-lan"] = s.AllowLan
	m["ipv6"] = s.IPv6
	if s.Mode != "" {
		m["mode"] = s.Mode
	}
	if s.LogLevel != "" {
		m["log-level"] = s.LogLevel
	}
	// the connections page groups by process, which needs it looked up
	// for every connection, not only those a PROCESS-NAME rule asks about
	if s.FindProcess {
		m["find-process-mode"] = "always"
	}
	// The app is the only controller: a profile's own (or a unix socket,
	// which takes no secret) must not open another way in.
	m["external-controller"] = ctl.Addr
	m["secret"] = ctl.Secret
	for _, k := range []string{"external-controller-unix", "external-controller-pipe", "external-controller-tls", "external-ui", "external-ui-url", "external-ui-name"} {
		delete(m, k)
	}

	tun, _ := m["tun"].(map[string]any)
	if tun == nil {
		tun = map[string]any{}
	}
	tun["enable"] = s.Tun
	if s.TunStack != "" {
		tun["stack"] = s.TunStack
	}
	tun["disable-icmp-forwarding"] = !s.ICMPForwarding
	if s.Tun {
		setDefault(tun, "auto-route", true)
		setDefault(tun, "auto-detect-interface", true)
		setDefault(tun, "dns-hijack", []any{"any:53", "tcp://any:53"})
	}
	m["tun"] = tun

	if pre := userRules(m, user); len(pre) > 0 {
		own, _ := m["rules"].([]any)
		m["rules"] = append(pre, own...)
	}
	return yaml.Marshal(m)
}

// The policies every profile has.
var builtin = map[string]bool{"DIRECT": true, "REJECT": true, "REJECT-DROP": true, "PASS": true, "COMPATIBLE": true}

// userRules is the user's rules that the profile can take: one naming a
// policy it doesn't have (added under another profile) would fail the
// whole configuration, so it is left out.
func userRules(m map[string]any, user []userrules.Rule) []any {
	if len(user) == 0 {
		return nil
	}
	have := map[string]bool{}
	for _, k := range []string{"proxies", "proxy-groups"} {
		list, _ := m[k].([]any)
		for _, p := range list {
			if p, ok := p.(map[string]any); ok {
				if name, ok := p["name"].(string); ok {
					have[name] = true
				}
			}
		}
	}
	var out []any
	for _, r := range user {
		if r.Check() == nil && (builtin[r.Policy] || have[r.Policy]) {
			out = append(out, r.String())
		}
	}
	return out
}

func setDefault(m map[string]any, k string, v any) {
	if _, ok := m[k]; !ok {
		m[k] = v
	}
}

// Write builds the configuration and writes it to path, readable by the
// user only (it holds the secret).
func Write(path string, profile []byte, s settings.Settings, ctl Controller, user []userrules.Rule) error {
	b, err := Build(profile, s, ctl, user)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
