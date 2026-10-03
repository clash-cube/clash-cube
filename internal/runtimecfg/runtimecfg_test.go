package runtimecfg

import (
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/localhost-copilot/mihomobar/internal/settings"
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
	out, err := Build(profile, s, Controller{Addr: "127.0.0.1:5555", Secret: "s3"})
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
	out, err = Build([]byte("tun: {disable-icmp-forwarding: false}"), s, Controller{})
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
