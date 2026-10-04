package runtimecfg

import (
	"reflect"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/localhost-copilot/clashcube/internal/modules"
	"github.com/localhost-copilot/clashcube/internal/settings"
)

func TestFixedRouteNodes(t *testing.T) {
	const name = "🇺🇸 US (01)"
	r := &modules.Route{Service: "Claude", Policy: "select", Pick: true, Nodes: map[string][]string{"p": {name, name}}}
	profile := []byte("proxies:\n  - {name: '" + name + "', type: ss}\nrules: [MATCH,DIRECT]\n")
	build := func(id string, profile []byte) map[string]any {
		t.Helper()
		body, err := Build(id, profile, settings.Defaults(), Controller{}, nil, []modules.Module{{Name: "Claude", Enabled: true, Route: r}})
		if err != nil {
			t.Fatal(err)
		}
		v, err := modules.Parse(string(body))
		if err != nil {
			t.Fatal(err)
		}
		return v["proxy-groups"].([]any)[0].(map[string]any)
	}
	g := build("p", profile)
	if !reflect.DeepEqual(g["proxies"], []any{name}) || g["filter"] != nil || g["include-all"] != nil {
		t.Fatalf("fixed picks were not emitted as a deduplicated literal list: %v", g)
	}
	// Picks from another profile must never turn into all nodes.
	if g := build("other", profile); !reflect.DeepEqual(g["proxies"], []any{"REJECT"}) {
		t.Fatalf("empty picks did not refuse connections: %v", g)
	}
	// A removed node cannot remain a dangling proxies reference. It may still
	// exist in a provider, so keep an exact filter that refuses absent matches.
	g = build("p", []byte("{}"))
	if g["proxies"] != nil || g["include-all"] != true || !strings.Contains(g["filter"].(string), `US\ \(01\)`) {
		t.Fatalf("missing picks did not become an exact dynamic selection: %v", g)
	}
}

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
	out, err := Build("p", []byte("rules: [\"MATCH,DIRECT\"]"), s, Controller{Addr: "127.0.0.1:5555", Secret: "s3"}, nil, mods)
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
	if _, err := Build("p", nil, s, Controller{}, nil, []modules.Module{{Name: "bad", Enabled: true, Body: "prepend-rules: x"}}); err == nil {
		t.Error("a bad module built")
	}
}

// A route's group never takes a name the profile has, nor one an earlier
// route took.
func TestRouteNames(t *testing.T) {
	r := &modules.Route{Service: "Google", Policy: "select"}
	mods := []modules.Module{{Name: "a", Enabled: true, Route: r}, {Name: "b", Enabled: true, Route: r}}
	out, err := Build("p", []byte("proxy-groups: [{name: Google, type: select, proxies: [DIRECT]}]\nrules: [\"MATCH,DIRECT\"]"), settings.Defaults(), Controller{}, nil, mods[:1])
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = yaml.Unmarshal(out, &m)
	gs := m["proxy-groups"].([]any)
	if len(gs) != 2 || gs[1].(map[string]any)["name"] != "Google"+modules.Suffix {
		t.Errorf("groups = %v", gs)
	}
	if rules := m["rules"].([]any); rules[0] != "GEOSITE,google,Google"+modules.Suffix {
		t.Errorf("rules = %v", rules)
	}
	if _, err := Build("p", nil, settings.Defaults(), Controller{}, nil, mods); err != nil {
		t.Fatal(err)
	}
}
