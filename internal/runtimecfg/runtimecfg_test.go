package runtimecfg

import (
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/localhost-copilot/mihomobar/internal/settings"
	"github.com/localhost-copilot/mihomobar/internal/userrules"
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
	out, err := Build(profile, s, Controller{Addr: "127.0.0.1:5555", Secret: "s3"}, nil)
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
	out, err = Build([]byte("tun: {disable-icmp-forwarding: false}"), s, Controller{}, nil)
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
	out, err := Build(profile, settings.Defaults(), Controller{}, user)
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
