package modules

import (
	"reflect"
	"testing"
)

func TestPortCheck(t *testing.T) {
	for _, bad := range []Port{
		{Port: 0},
		{Port: 70000},
		{Port: 7891, Listen: "192.168.1.2"},
		{Port: 7891, Target: map[string]string{"": "a"}},
		{Port: 7891, Target: map[string]string{"p": " "}},
		{Port: 7891, Target: map[string]string{"p": "a\nb"}},
		{Port: 7891, User: "u"},
		{Port: 7891, Pass: "p"},
		{Port: 7891, User: "u\n", Pass: "p"},
	} {
		if bad.Check() == nil {
			t.Errorf("%+v passed", bad)
		}
	}
	for _, ok := range []Port{{Port: 7891}, {Port: 7891, Listen: "0.0.0.0", User: "u", Pass: "p", Target: map[string]string{"p": "a"}}} {
		if err := ok.Check(); err != nil {
			t.Errorf("%+v: %v", ok, err)
		}
	}
	if (Module{Name: "both", Route: &Route{Service: "Google", Policy: "DIRECT"}, Port: &Port{Port: 7891}}).Check() == nil {
		t.Error("a module of two kinds passed")
	}
}

func TestPortGenerate(t *testing.T) {
	config := map[string]any{
		"proxies":      []any{map[string]any{"name": "declared"}},
		"proxy-groups": []any{map[string]any{"name": "Group"}},
	}
	gen := func(p Port) map[string]any {
		t.Helper()
		body, err := p.Generate("p", config)
		if err != nil {
			t.Fatal(err)
		}
		m, err := Parse(body)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	if m := gen(Port{Port: 7891, Target: map[string]string{"other": "declared"}}); m != nil {
		t.Errorf("no node for the profile, yet %v", m)
	}
	m := gen(Port{Port: 7891, Target: map[string]string{"p": "declared"}})
	want := map[string]any{"name": "ClashCube port 7891", "type": "socks", "listen": "127.0.0.1", "port": 7891, "udp": false, "users": []any{}, "proxy": "declared"}
	if l := m["listeners+"].([]any)[0]; !reflect.DeepEqual(l, want) || m["append-proxy-groups"] != nil {
		t.Errorf("declared: %v", m)
	}
	m = gen(Port{Port: 7891, Target: map[string]string{"p": "Group"}, Listen: "0.0.0.0", UDP: true, User: "u", Pass: "p"})
	l := m["listeners+"].([]any)[0].(map[string]any)
	if l["proxy"] != "Group" || l["listen"] != "0.0.0.0" || l["udp"] != true || !reflect.DeepEqual(l["users"], []any{map[string]any{"username": "u", "password": "p"}}) {
		t.Errorf("group: %v", l)
	}
	// a provider's node, or one that is gone, through a hidden group that
	// takes it by name alone
	m = gen(Port{Port: 7891, Target: map[string]string{"p": "US (1)`x"}})
	l = m["listeners+"].([]any)[0].(map[string]any)
	g := m["append-proxy-groups"].([]any)[0].(map[string]any)
	if l["proxy"] != ChainPrefix+"port 7891" || g["name"] != l["proxy"] || g["hidden"] != true || g["empty-fallback"] != "REJECT" || g["filter"] != `^US\ \(1\)\x60x$` {
		t.Errorf("provider: %v", m)
	}
}

func TestPortProfiles(t *testing.T) {
	p := &Port{Port: 7891, Target: map[string]string{"a": "x", "b": "y"}}
	if c := p.Only("a", "c").(*Port); !reflect.DeepEqual(c.Target, map[string]string{"c": "x"}) || len(p.Target) != 2 {
		t.Errorf("only: %v, left %v", c.Target, p.Target)
	}
	if !p.CopyProfile("b", "d") || p.Target["d"] != "y" || p.CopyProfile("z", "e") {
		t.Errorf("copy: %v", p.Target)
	}
	if !p.ForgetProfile("a") || p.ForgetProfile("a") || len(p.Target) != 2 {
		t.Errorf("forget: %v", p.Target)
	}
}

func TestCheckPorts(t *testing.T) {
	port := func(name, profile string, n int, on bool) Module {
		return Module{Name: name, Profile: profile, Enabled: on, Port: &Port{Port: n}}
	}
	for _, tc := range []struct {
		ms []Module
		ok bool
	}{
		{[]Module{port("a", "", 7891, true), port("b", "", 7892, true)}, true},
		{[]Module{port("a", "", 7891, true), port("b", "", 7891, false)}, true},
		{[]Module{port("a", "p", 7891, true), port("b", "q", 7891, true)}, true},
		{[]Module{port("a", "", 7891, true), port("b", "", 7891, true)}, false},
		{[]Module{port("a", "", 7891, true), port("b", "q", 7891, true)}, false},
		{[]Module{port("a", "p", 7891, true), port("b", "p", 7891, true)}, false},
	} {
		if err := checkPorts(tc.ms); (err == nil) != tc.ok {
			t.Errorf("%+v: %v", tc.ms, err)
		}
	}
}

func TestOrderedPutsTrailingLast(t *testing.T) {
	ms := []Module{
		{Name: "port", Port: &Port{Port: 1}},
		{Name: "yaml"},
		{Name: "route", Route: &Route{Service: "Google", Policy: "DIRECT"}},
	}
	var names []string
	for _, m := range Ordered(ms) {
		names = append(names, m.Name)
	}
	if !reflect.DeepEqual(names, []string{"yaml", "route", "port"}) {
		t.Errorf("order = %v", names)
	}
}

func TestWasKeepsTheForm(t *testing.T) {
	t.Setenv("CLASHCUBE_HOME", t.TempDir())
	port := &Module{Name: "old", Body: "x: 1", Port: &Port{Port: 7891, Target: map[string]string{"a": "x"}}}
	if err := Save([]Module{{Name: "yaml", Body: "a: 1", Was: port}}); err != nil {
		t.Fatal(err)
	}
	m := List()[0]
	if m.Kind() != nil || m.Was == nil || m.Was.Port == nil || m.Was.Port.Port != 7891 || m.Was.Name != "" || m.Was.Body != "" {
		t.Fatalf("got %+v, was %+v", m, m.Was)
	}
	if err := CopyProfile("a", "b"); err != nil || List()[0].Was.Port.Target["b"] != "x" {
		t.Errorf("copy: %v %+v", err, List()[0].Was.Port)
	}
	if err := ForgetProfile("a"); err != nil || List()[0].Was.Port.Target["a"] != "" {
		t.Errorf("forget: %v %+v", err, List()[0].Was.Port)
	}
	for _, bad := range []Module{
		{Name: "both", Port: &Port{Port: 1, Target: map[string]string{"a": "x"}}, Was: port},
		{Name: "empty", Body: "a: 1", Was: &Module{}},
		{Name: "nested", Body: "a: 1", Was: &Module{Port: port.Port, Was: port}},
	} {
		if Save([]Module{bad}) == nil {
			t.Errorf("%s saved", bad.Name)
		}
	}
}
