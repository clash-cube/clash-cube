// Package runtimecfg writes the configuration the core runs: the profile in
// use with the app's settings laid over it, so the core starts in the state
// the user asked for instead of being patched into it afterwards.
package runtimecfg

import (
	"fmt"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"

	"github.com/localhost-copilot/mihomobar/internal/modules"
	"github.com/localhost-copilot/mihomobar/internal/settings"
	"github.com/localhost-copilot/mihomobar/internal/userrules"
)

// Controller is where the core's API listens and the secret it takes.
type Controller struct {
	Addr   string
	Secret string
}

// Build is profile (of that ID) with the enabled modules merged over it in
// order, then s and ctl, which win over both, and the user's rules ahead
// of all others.
func Build(id string, profile []byte, s settings.Settings, ctl Controller, user []userrules.Rule, mods []modules.Module) ([]byte, error) {
	var m map[string]any
	if err := yaml.Unmarshal(profile, &m); err != nil {
		return nil, fmt.Errorf("profile: %w", err)
	}
	if m == nil {
		m = map[string]any{}
	}
	for _, mod := range mods {
		if !mod.Enabled {
			continue
		}
		body := mod.Body
		if mod.Route != nil {
			have := policies(m)
			b, err := mod.Route.Body(id, func(n string) bool { return have[n] || builtin[n] })
			if err != nil {
				return nil, fmt.Errorf("module %s: %w", mod.Name, err)
			}
			body = b
		}
		v, err := modules.Parse(body)
		if err == nil {
			err = Merge(m, v)
		}
		if err != nil {
			return nil, fmt.Errorf("module %s: %w", mod.Name, err)
		}
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
	guard(m, tun, s)

	pre := userRules(m, user)
	if s.BlockSTUN {
		pre = append([]any{stunRule}, pre...)
	}
	if len(pre) > 0 {
		own, _ := m["rules"].([]any)
		m["rules"] = append(pre, own...)
	}
	return yaml.Marshal(m)
}

// stunRule rejects STUN over UDP. A node without UDP makes mihomo skip
// its rule and go on matching, which can end at DIRECT and show WebRTC
// this Mac's address; rejected, WebRTC falls back to TURN over TCP.
const stunRule = "AND,((NETWORK,UDP),(DST-PORT,3478/5349/19302-19309)),REJECT"

// The dns section laid down when a profile has none.
var defaultNameservers = []any{"https://dns.alidns.com/dns-query", "https://doh.pub/dns-query"}

// guard applies the leak protection settings.
func guard(m, tun map[string]any, s settings.Settings) {
	dns, _ := m["dns"].(map[string]any)
	if dns == nil {
		dns = map[string]any{}
	}
	// mihomo drops the TUN's IPv6 address when ipv6 is off, and auto-route
	// only routes IPv6 into a TUN that has one, so IPv6 would go around
	// it. Keep ipv6 on for the core and stop AAAA answers in dns instead,
	// which needs the core's dns on.
	v6 := s.GuardIPv6 && s.Tun && !s.IPv6
	if v6 {
		m["ipv6"] = true
		dns["ipv6"] = false
	}
	if s.GuardDNS || s.DNSRespectRules || v6 {
		if dns["enable"] != true {
			dns["enable"] = true
			setDefault(dns, "enhanced-mode", "fake-ip")
			setDefault(dns, "fake-ip-range", "198.18.0.1/16")
		}
		if ns, _ := dns["nameserver"].([]any); len(ns) == 0 {
			dns["nameserver"] = defaultNameservers
		}
	}
	if s.GuardDNS && s.Tun {
		tun["dns-hijack"] = []any{"any:53", "tcp://any:53"}
	}
	if s.DNSRespectRules {
		dns["respect-rules"] = true
		// required by respect-rules: proxy servers' names are looked up
		// direct, so the core can reach them before it has a route
		if ps, _ := dns["proxy-server-nameserver"].([]any); len(ps) == 0 {
			dns["proxy-server-nameserver"] = dns["nameserver"]
		}
	}
	if len(dns) > 0 {
		m["dns"] = dns
	}
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
	have := policies(m)
	var out []any
	for _, r := range user {
		if r.Check() == nil && (builtin[r.Policy] || have[r.Policy]) {
			out = append(out, r.String())
		}
	}
	return out
}

// policies is the names of m's proxies and groups.
func policies(m map[string]any) map[string]bool {
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
	return have
}

func setDefault(m map[string]any, k string, v any) {
	if _, ok := m[k]; !ok {
		m[k] = v
	}
}

// Write builds the configuration and writes it to path, readable by the
// user only (it holds the secret).
func Write(path, id string, profile []byte, s settings.Settings, ctl Controller, user []userrules.Rule, mods []modules.Module) error {
	b, err := Build(id, profile, s, ctl, user, mods)
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
