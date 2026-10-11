package modules

import (
	"reflect"
	"testing"
)

func TestAppsCheck(t *testing.T) {
	app := []App{{Type: "PROCESS-NAME", Payload: "claude"}}
	for _, bad := range []Apps{
		{},
		{Apps: []App{{Type: "DOMAIN", Payload: "a.com"}}},
		{Apps: []App{{Type: "PROCESS-NAME", Payload: " "}}},
		{Apps: []App{{Type: "PROCESS-NAME", Payload: "a,DIRECT"}}},
		{Apps: []App{{Type: "PROCESS-PATH-REGEX", Payload: "^a\nMATCH"}}},
		{Apps: app, Target: map[string]string{"": "a"}},
		{Apps: app, Target: map[string]string{"p": "a,b"}},
	} {
		if bad.Check() == nil {
			t.Errorf("%+v passed", bad)
		}
	}
	if err := (&Apps{Apps: app, Target: map[string]string{"p": "US 01"}}).Check(); err != nil {
		t.Error(err)
	}
}

func TestAppsGenerate(t *testing.T) {
	config := map[string]any{
		"proxies":      []any{map[string]any{"name": "declared"}},
		"proxy-groups": []any{map[string]any{"name": "Group"}},
	}
	apps := []App{{Type: "PROCESS-NAME", Payload: "claude"}, {Type: "PROCESS-PATH-REGEX", Payload: `^/Applications/Claude\.app/`}}
	gen := func(a Apps) map[string]any {
		t.Helper()
		body, err := a.Generate("p", config)
		if err != nil {
			t.Fatal(err)
		}
		m, err := Parse(body)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	if m := gen(Apps{Apps: apps, Target: map[string]string{"other": "Group"}}); m != nil {
		t.Errorf("no node for the profile, yet %v", m)
	}
	m := gen(Apps{Apps: apps, Target: map[string]string{"p": "Group"}})
	want := []any{"PROCESS-NAME,claude,Group", `PROCESS-PATH-REGEX,^/Applications/Claude\.app/,Group`, "MATCH,DIRECT"}
	if !reflect.DeepEqual(m["rules!"], want) || m["append-proxy-groups"] != nil {
		t.Errorf("group: %v", m)
	}
	// a provider's node, or one that is gone, through a hidden group that
	// takes it by name alone
	m = gen(Apps{Apps: apps[:1], Target: map[string]string{"p": "US (1)"}})
	g := m["append-proxy-groups"].([]any)[0].(map[string]any)
	if !reflect.DeepEqual(m["rules!"], []any{"PROCESS-NAME,claude," + AppsGroup, "MATCH,DIRECT"}) ||
		g["name"] != AppsGroup || g["hidden"] != true || g["empty-fallback"] != "REJECT" || g["filter"] != `^US\ \(1\)$` {
		t.Errorf("provider: %v", m)
	}
}

func TestAppsProfiles(t *testing.T) {
	a := &Apps{Apps: []App{{Type: "PROCESS-NAME", Payload: "x"}}, Target: map[string]string{"a": "x", "b": "y"}}
	if c := a.Only("a", "c").(*Apps); !reflect.DeepEqual(c.Target, map[string]string{"c": "x"}) || len(a.Target) != 2 || len(c.Apps) != 1 {
		t.Errorf("only: %+v, left %v", c, a.Target)
	}
	if !a.CopyProfile("b", "d") || a.Target["d"] != "y" || a.CopyProfile("z", "e") {
		t.Errorf("copy: %v", a.Target)
	}
	if !a.ForgetProfile("a") || a.ForgetProfile("a") || len(a.Target) != 2 {
		t.Errorf("forget: %v", a.Target)
	}
	if !Trailing(a) {
		t.Error("apps not laid last")
	}
}

func TestCheckApps(t *testing.T) {
	apps := func(name, profile string, on bool) Module {
		return Module{Name: name, Profile: profile, Enabled: on, Apps: &Apps{Apps: []App{{Type: "PROCESS-NAME", Payload: "x"}}}}
	}
	for _, tc := range []struct {
		ms []Module
		ok bool
	}{
		{[]Module{apps("a", "", true), apps("b", "", false)}, true},
		{[]Module{apps("a", "p", true), apps("b", "q", true)}, true},
		{[]Module{apps("a", "", true), apps("b", "", true)}, false},
		{[]Module{apps("a", "", true), apps("b", "q", true)}, false},
		{[]Module{apps("a", "p", true), apps("b", "p", true)}, false},
	} {
		if err := checkApps(tc.ms); (err == nil) != tc.ok {
			t.Errorf("%+v: %v", tc.ms, err)
		}
	}
}
