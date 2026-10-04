package modules

import (
	"reflect"
	"strings"
	"testing"
)

func TestSaveAndCheck(t *testing.T) {
	t.Setenv("MIHOMOBAR_HOME", t.TempDir())
	if len(List()) != 0 {
		t.Fatal("modules before any were saved")
	}
	ms := []Module{{Name: " DNS ", Enabled: true, Body: "dns: {enable: true}\n+rules: []\nsecret: x"}, {Name: "Empty"}}
	if err := Save(ms); err != nil {
		t.Fatal(err)
	}
	got := List()
	if len(got) != 2 || got[0].Name != "DNS" || got[0].ID == "" || got[0].ID == got[1].ID {
		t.Fatalf("got %+v", got)
	}
	if k := got[0].Keys(); !reflect.DeepEqual(k, []string{"+rules", "dns"}) {
		t.Errorf("keys = %v", k)
	}
	for _, bad := range []Module{{Name: "", Body: ""}, {Name: "list", Body: "- a"}, {Name: "yaml", Body: "a: [b"}} {
		if Save([]Module{bad}) == nil {
			t.Errorf("%+v saved", bad)
		}
	}
	if len(List()) != 2 {
		t.Error("a refused save changed the modules")
	}
}

func TestRoute(t *testing.T) {
	body, err := Route{Service: "Telegram", Policy: "select", Region: "hk"}.Body("p", nil)
	if err != nil {
		t.Fatal(err)
	}
	v, _ := Parse(body)
	rules := v["prepend-rules"].([]any)
	if rules[0] != "GEOSITE,telegram,Telegram" || rules[1] != "IP-CIDR,91.108.4.0/22,Telegram,no-resolve" {
		t.Errorf("rules = %v", rules)
	}
	g := v["append-proxy-groups"].([]any)[0].(map[string]any)
	if g["name"] != "Telegram" || g["type"] != "select" || g["include-all"] != true || g["filter"] == nil {
		t.Errorf("group = %v", g)
	}
	// a name the profile has already is set apart
	body, _ = Route{Service: "Google", Policy: "url-test"}.Body("p", func(n string) bool { return n == "Google" })
	if v, _ := Parse(body); v["append-proxy-groups"].([]any)[0].(map[string]any)["name"] != "Google"+Suffix {
		t.Errorf("body = %s", body)
	}
	body, _ = Route{Service: "Google", Policy: "DIRECT"}.Body("p", nil)
	if v, _ := Parse(body); v["append-proxy-groups"] != nil || v["prepend-rules"].([]any)[0] != "GEOSITE,google,DIRECT" {
		t.Errorf("body = %s", body)
	}
	if g := mustGroup(t, Route{Service: "Google", Policy: "select"}); g["empty-fallback"] != "REJECT" || g["filter"] != nil {
		t.Errorf("group = %v", g)
	}
	for _, bad := range []Route{
		{Service: "Nope", Policy: "select"}, {Service: "Google", Policy: "fallback"}, {Service: "Google", Policy: "select", Region: "mars"},
		{Service: "Google", Policy: "select", Region: "hk", Pick: true},
		{Service: "Google", Policy: "select", Keywords: []string{"a"}},
		{Service: "Google", Policy: "select", Pick: true, Nodes: map[string][]string{"p": {" "}}},
		{Service: "Google", Policy: "select", Pick: true, Nodes: map[string][]string{"": {"a"}}},
		{Service: "Google", Policy: "select", Pick: true, Keywords: []string{"a\nb"}},
	} {
		if Save([]Module{{Name: "x", Route: &bad}}) == nil {
			t.Errorf("%+v saved", bad)
		}
	}
}

func TestRegions(t *testing.T) {
	cases := map[string][]string{
		"hk": {"🇭🇰 香港 01", "HK-02", "Hong Kong IPLC"},
		"us": {"美国 洛杉矶", "US 01", "United States", "🇺🇸 Los Angeles 01"},
		"jp": {"日本东京", "JP|03"},
	}
	not := map[string][]string{"us": {"Russia 01", "Australia"}, "hk": {"Thkland"}, "tw": {"平台 01"}}
	for _, r := range Regions {
		for _, n := range cases[r.Key] {
			if !r.Matches(n) {
				t.Errorf("%s doesn't take %q", r.Key, n)
			}
		}
		for _, n := range not[r.Key] {
			if r.Matches(n) {
				t.Errorf("%s takes %q", r.Key, n)
			}
		}
	}
}

func mustGroup(t *testing.T, r Route) map[string]any {
	t.Helper()
	body, err := r.Body("p", nil)
	if err != nil {
		t.Fatal(err)
	}
	v, _ := Parse(body)
	return v["append-proxy-groups"].([]any)[0].(map[string]any)
}

// Picked nodes match whole, as written, and only in their profile;
// keywords anywhere, in any case, in every profile.
func TestRouteTakes(t *testing.T) {
	r := Route{Service: "Google", Policy: "select", Pick: true,
		Nodes: map[string][]string{"p": {"🇭🇰 HK (IPLC) 01", "a|b", "x`y"}, "q": {"US 01"}}, Keywords: []string{"专线", "iplc"}}
	for name, want := range map[string]bool{
		"🇭🇰 HK (IPLC) 01": true, "🇭🇰 HK (IPLC) 012": true, // the keyword
		"a|b": true, "a": false, "b": false, "x`y": true,
		"JP 专线 02": true, "US IPLC": true, "US 01": false,
	} {
		if got := r.Takes("p", name); got != want {
			t.Errorf("%q: %v, want %v", name, got, want)
		}
	}
	if !r.Takes("q", "US 01") || r.Takes("q", "a|b") || !r.Takes("other", "JP 专线") || r.Takes("other", "US 01") {
		t.Error("picks crossed profiles")
	}
	if f := mustGroup(t, r)["filter"].(string); strings.Contains(f, "`") {
		t.Errorf("filter %q has a backtick, which mihomo splits at", f)
	}
	only := Route{Service: "Google", Policy: "select", Pick: true, Nodes: map[string][]string{"p": {"US 01"}}}
	if !only.Takes("p", "US 01") || only.Takes("p", "US 012") || only.Takes("p", "XUS 01") {
		t.Error("a picked name matched more than itself")
	}
	// a profile nothing was picked for takes no node, rather than all
	if only.Takes("q", "US 01") || only.filter("q") != none {
		t.Error("a profile with no picks took a node")
	}
}

func TestPicksFollowProfiles(t *testing.T) {
	t.Setenv("MIHOMOBAR_HOME", t.TempDir())
	r := &Route{Service: "Google", Policy: "select", Pick: true, Nodes: map[string][]string{"a": {"n1"}, "b": {"n2"}}}
	if err := Save([]Module{{Name: "Google", Route: r}, {Name: "plain", Body: "hosts: {}"}}); err != nil {
		t.Fatal(err)
	}
	if err := CopyPicks("a", "c"); err != nil {
		t.Fatal(err)
	}
	if err := ForgetPicks("b"); err != nil {
		t.Fatal(err)
	}
	got := List()[0].Route.Nodes
	if !reflect.DeepEqual(got, map[string][]string{"a": {"n1"}, "c": {"n1"}}) {
		t.Errorf("nodes = %v", got)
	}
}
