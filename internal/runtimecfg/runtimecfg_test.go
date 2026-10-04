package runtimecfg

import (
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/localhost-copilot/clashcube/internal/settings"
	"github.com/localhost-copilot/clashcube/internal/userrules"
)

func TestBuildOverlays(t *testing.T) {
	profile := []byte(`
mixed-port: 1234
external-controller: 0.0.0.0:9090
external-controller-unix: /tmp/x.sock
secret: weak
tun:
  enable: true
  device: utun9
proxies: []
rules: [MATCH,DIRECT]
custom-key: kept
`)
	s := settings.Defaults()
	s.Mode = "global"
	out, err := Build("p", profile, s, Controller{Addr: "127.0.0.1:5555", Secret: "s3"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := yaml.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"mixed-port": 7890, "external-controller": "127.0.0.1:5555", "secret": "s3", "mode": "global", "custom-key": "kept"}
	for k, v := range want {
		if m[k] != v {
			t.Errorf("%s = %v, want %v", k, m[k], v)
		}
	}
	if _, ok := m["external-controller-unix"]; ok {
		t.Error("unix controller kept")
	}
	tun := m["tun"].(map[string]any)
	if tun["enable"] != false || tun["device"] != "utun9" || tun["disable-icmp-forwarding"] != false {
		t.Errorf("tun = %v", tun)
	}

	s.ICMPForwarding = false
	out, err = Build("p", []byte("tun: {disable-icmp-forwarding: false}"), s, Controller{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m = nil
	if err := yaml.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if v := m["tun"].(map[string]any)["disable-icmp-forwarding"]; v != true {
		t.Errorf("disable-icmp-forwarding = %v, want true", v)
	}
}

// The user's rules go first; one whose policy the profile lacks is left
// out rather than failing the profile.
func TestBuildUserRules(t *testing.T) {
	profile := []byte(`
proxies: [{name: hk, type: direct}]
proxy-groups: [{name: Proxy, type: select, proxies: [hk]}]
rules:
  - MATCH,Proxy
`)
	user := []userrules.Rule{
		{Type: "DOMAIN", Payload: "a.com", Policy: "Proxy"},
		{Type: "DOMAIN", Payload: "b.com", Policy: "Gone"},
		{Type: "DOMAIN-SUFFIX", Payload: "lan", Policy: "DIRECT"},
	}
	out, err := Build("p", profile, settings.Defaults(), Controller{}, user, nil)
	if err != nil {
		t.Fatal(err)
	}
	var m struct{ Rules []string }
	if err := yaml.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	want := []string{"DOMAIN,a.com,Proxy", "DOMAIN-SUFFIX,lan,DIRECT", "MATCH,Proxy"}
	if len(m.Rules) != len(want) {
		t.Fatalf("rules = %v", m.Rules)
	}
	for i := range want {
		if m.Rules[i] != want[i] {
			t.Errorf("rules = %v, want %v", m.Rules, want)
			break
		}
	}
}

func build(t *testing.T, profile string, s settings.Settings) map[string]any {
	t.Helper()
	out, err := Build("p", []byte(profile), s, Controller{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := yaml.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// Off by default: the profile's dns and rules are left as they are.
func TestGuardOff(t *testing.T) {
	m := build(t, "tun: {dns-hijack: []}\nrules: [\"MATCH,DIRECT\"]", settings.Defaults())
	if _, ok := m["dns"]; ok {
		t.Errorf("dns = %v", m["dns"])
	}
	if m["ipv6"] != false || len(m["rules"].([]any)) != 1 {
		t.Errorf("ipv6 = %v, rules = %v", m["ipv6"], m["rules"])
	}
}

func TestGuard(t *testing.T) {
	s := settings.Defaults()
	s.Tun, s.GuardIPv6, s.GuardDNS, s.BlockSTUN, s.DNSRespectRules = true, true, true, true, true
	m := build(t, "dns: {enable: false, nameserver: [1.1.1.1]}\ntun: {dns-hijack: []}\nrules: [\"MATCH,DIRECT\"]", s)
	dns := m["dns"].(map[string]any)
	if m["ipv6"] != true || dns["ipv6"] != false {
		t.Errorf("ipv6 = %v, dns.ipv6 = %v", m["ipv6"], dns["ipv6"])
	}
	if dns["enable"] != true || dns["enhanced-mode"] != "fake-ip" || dns["respect-rules"] != true {
		t.Errorf("dns = %v", dns)
	}
	if ps := dns["proxy-server-nameserver"].([]any); len(ps) != 1 || ps[0] != "1.1.1.1" {
		t.Errorf("proxy-server-nameserver = %v", ps)
	}
	if h := m["tun"].(map[string]any)["dns-hijack"].([]any); len(h) != 2 {
		t.Errorf("dns-hijack = %v", h)
	}
	if r := m["rules"].([]any); len(r) != 2 || r[0] != stunRule {
		t.Errorf("rules = %v", r)
	}

	// IPv6 on, or no TUN: nothing to route, ipv6 is the user's
	s.IPv6 = true
	if m := build(t, "", s); m["dns"].(map[string]any)["ipv6"] != nil {
		t.Errorf("dns.ipv6 set with IPv6 on")
	}
	s.IPv6, s.Tun = false, false
	if m := build(t, "", s); m["ipv6"] != false {
		t.Errorf("ipv6 = %v without TUN", m["ipv6"])
	}

	// a profile without dns gets a working one
	s = settings.Defaults()
	s.Tun, s.GuardIPv6 = true, true
	dns = build(t, "", s)["dns"].(map[string]any)
	if dns["enable"] != true || len(dns["nameserver"].([]any)) == 0 {
		t.Errorf("dns = %v", dns)
	}
}
