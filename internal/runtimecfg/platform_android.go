package runtimecfg

import "github.com/localhost-copilot/clashcube/internal/settings"

func MixedPort(settings.Settings) int { return 0 }

func platformPolicy(m, tun map[string]any, s settings.Settings) {
	for _, key := range []string{"port", "socks-port", "mixed-port", "redir-port", "tproxy-port"} {
		m[key] = 0
	}
	// Profiles cannot add unauthenticated listeners on the shared device loopback.
	delete(m, "listeners")
	m["allow-lan"] = false
	m["interface-name"] = ""
	m["routing-mark"] = 0
	m["find-process-mode"] = "always" // installed before parsing by libclashcore
	m["ipv6"] = s.IPv6
	tun["enable"] = true
	tun["auto-route"] = false
	tun["auto-detect-interface"] = false
	tun["auto-redirect"] = false
	delete(tun, "file-descriptor")
	tun["dns-hijack"] = []any{"any:53", "tcp://any:53"}
	dns, _ := m["dns"].(map[string]any)
	if dns == nil {
		dns = map[string]any{}
	}
	dns["enable"] = true
	dns["ipv6"] = s.IPv6
	if ns, _ := dns["nameserver"].([]any); len(ns) == 0 {
		dns["nameserver"] = defaultNameservers
	}
	m["dns"] = dns
	ntp, _ := m["ntp"].(map[string]any)
	if ntp == nil {
		ntp = map[string]any{}
	}
	ntp["write-to-system"] = false
	m["ntp"] = ntp
}
