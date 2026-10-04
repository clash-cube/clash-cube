package runtimecfg

import (
	"reflect"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/localhost-copilot/mihomobar/internal/modules"
	"github.com/localhost-copilot/mihomobar/internal/settings"
)

func TestMerge(t *testing.T) {
	m := map[string]any{
		"dns":   map[string]any{"enable": true, "fake-ip-filter": []any{"a"}, "nameserver": []any{"1.1.1.1"}, "nameserver-policy": map[string]any{"x": "y"}},
		"hosts": map[string]any{"old": "1.2.3.4"},
		"rules": []any{"MATCH,DIRECT"},
	}
	mod := map[string]any{
		"dns": map[string]any{
			"+fake-ip-filter":    []any{"b"},
			"nameserver":         []any{"223.5.5.5"},
			"nameserver-policy!": map[string]any{"z": "w"},
		},
		"hosts":         map[string]any{"new": "5.6.7.8"},
		"prepend-rules": []any{"DOMAIN,a.com,DIRECT"},
		"rules+":        []any{"DOMAIN,z.com,REJECT"},
		"sniffer":       map[string]any{"enable": true},
	}
	if err := Merge(m, mod); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"dns":     map[string]any{"enable": true, "fake-ip-filter": []any{"b", "a"}, "nameserver": []any{"223.5.5.5"}, "nameserver-policy": map[string]any{"z": "w"}},
		"hosts":   map[string]any{"old": "1.2.3.4", "new": "5.6.7.8"},
		"rules":   []any{"DOMAIN,a.com,DIRECT", "MATCH,DIRECT", "DOMAIN,z.com,REJECT"},
		"sniffer": map[string]any{"enable": true},
	}
	if !reflect.DeepEqual(m, want) {
		t.Errorf("got  %v\nwant %v", m, want)
	}
	// a section the profile lacks is made, its +keys read
	m = map[string]any{}
	if err := Merge(m, map[string]any{"dns": map[string]any{"+fake-ip-filter": []any{"+.lan"}, "nameserver-policy!": map[string]any{"a": "b"}}}); err != nil {
		t.Fatal(err)
	}
	if want := map[string]any{"dns": map[string]any{"fake-ip-filter": []any{"+.lan"}, "nameserver-policy": map[string]any{"a": "b"}}}; !reflect.DeepEqual(m, want) {
		t.Errorf("got %v, want %v", m, want)
	}
	for _, bad := range []map[string]any{
		{"prepend-rules": "MATCH,DIRECT"},
		{"+hosts": []any{"x"}},
	} {
		if Merge(map[string]any{"hosts": map[string]any{}}, bad) == nil {
			t.Errorf("%v merged", bad)
		}
	}
}

// Modules go under the app's settings: they can't move the controller,
// the ports or TUN; disabled ones are skipped.
func TestBuildModules(t *testing.T) {
	s := settings.Defaults()
	mods := []modules.Module{
		{Name: "dns", Enabled: true, Body: "dns: {enable: true}\nprepend-rules: [\"DOMAIN,a.com,DIRECT\"]"},
		{Name: "sneaky", Enabled: true, Body: "external-controller: 0.0.0.0:9090\nexternal-controller-unix: /tmp/x\nsecret: ''\nmixed-port: 1\ntun: {enable: true, device: utun7}"},
		{Name: "off", Enabled: false, Body: "hosts: {a: 1.1.1.1}"},
	}
	out, err := Build([]byte("rules: [\"MATCH,DIRECT\"]"), s, Controller{Addr: "127.0.0.1:5555", Secret: "s3"}, nil, mods)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := yaml.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["external-controller"] != "127.0.0.1:5555" || m["secret"] != "s3" || m["mixed-port"] != 7890 {
		t.Errorf("controller or port taken: %v %v %v", m["external-controller"], m["secret"], m["mixed-port"])
	}
	if _, ok := m["external-controller-unix"]; ok {
		t.Error("unix controller kept")
	}
	if tun := m["tun"].(map[string]any); tun["enable"] != false || tun["device"] != "utun7" {
		t.Errorf("tun = %v", tun)
	}
	if _, ok := m["hosts"]; ok {
		t.Error("a disabled module merged")
	}
	if r := m["rules"].([]any); len(r) != 2 || r[0] != "DOMAIN,a.com,DIRECT" {
		t.Errorf("rules = %v", r)
	}
	if _, err := Build(nil, s, Controller{}, nil, []modules.Module{{Name: "bad", Enabled: true, Body: "prepend-rules: x"}}); err == nil {
		t.Error("a bad module built")
	}
}
