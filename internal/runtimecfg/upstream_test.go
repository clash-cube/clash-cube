package runtimecfg

import (
	"reflect"
	"strings"
	"testing"

	"github.com/dlclark/regexp2"
	"github.com/localhost-copilot/clashcube/internal/modules"
	"github.com/localhost-copilot/clashcube/internal/settings"
	"go.yaml.in/yaml/v3"
)

func TestUpstreamIsolation(t *testing.T) {
	profile := []byte(`proxies:
  - {name: front, type: socks5, server: localhost, port: 1001}
  - {name: exit, type: socks5, server: localhost, port: 1002}
proxy-groups:
  - {name: original, type: select, include-all: true}
proxy-providers:
  remote: {type: http, url: 'https://example.com/nodes', path: providers/original.yaml, override: {additional-prefix: 'US '}}
`)
	r := modules.Route{Service: "Google", Policy: "select", Pick: true, Nodes: map[string][]string{"p": {"exit"}}, Upstream: map[string]string{"p": "front"}}
	body, err := Build("p", profile, settings.Defaults(), Controller{}, nil, []modules.Module{{Name: "Google", Enabled: true, Route: &r}})
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := yaml.Unmarshal(body, &config); err != nil {
		t.Fatal(err)
	}
	nodes := config["proxies"].([]any)
	if len(nodes) != 3 {
		t.Fatalf("nodes = %v", nodes)
	}
	for _, node := range nodes[:2] {
		if node.(map[string]any)["dialer-proxy"] != nil {
			t.Fatal("modified original node")
		}
	}
	clone := nodes[2].(map[string]any)
	if NodeLabel(clone["name"].(string)) != "exit" || clone["dialer-proxy"] == nil {
		t.Fatalf("clone = %v", clone)
	}
	groups := config["proxy-groups"].([]any)
	original := groups[0].(map[string]any)
	excluded, _ := regexp2.MustCompile(original["exclude-filter"].(string), 0).MatchString(clone["name"].(string))
	if !excluded {
		t.Fatal("original include-all group can select private copies")
	}
	g := groups[2].(map[string]any)
	if !reflect.DeepEqual(g["proxies"], []any{clone["name"]}) {
		t.Fatalf("group = %v", g)
	}
	providers := config["proxy-providers"].(map[string]any)
	for name, value := range providers {
		if !strings.HasPrefix(name, ChainPrefix) {
			continue
		}
		provider := value.(map[string]any)
		if provider["path"] == "providers/original.yaml" {
			t.Fatal("private provider shares a writable cache")
		}
		if provider["override"].(map[string]any)["additional-prefix"] != "US " {
			t.Fatal("lost provider override")
		}
	}
	// An upstream belongs to a profile; it must not leak into another profile.
	body, err = Build("other", profile, settings.Defaults(), Controller{}, nil, []modules.Module{{Name: "Google", Enabled: true, Route: &r}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "dialer-proxy:") {
		t.Fatal("upstream crossed profiles")
	}
}

func TestPrivateNodeFilters(t *testing.T) {
	prefix := ChainPrefix + "US keyword/nodes/"
	for _, tc := range []struct {
		r    modules.Route
		name string
		want bool
	}{
		{modules.Route{Region: "us"}, "US 01", true},
		{modules.Route{Region: "us"}, "Russia", false},
		{modules.Route{Pick: true, Keywords: []string{"keyword"}}, "HK 01", false},
		{modules.Route{Pick: true, Keywords: []string{"keyword"}}, "HK keyword", true},
		{modules.Route{Pick: true, Nodes: map[string][]string{"p": {"a^b`(1)"}}}, "a^b`(1)", true},
		{modules.Route{Pick: true, Nodes: map[string][]string{"p": {"a^b`(1)"}}}, "a^b`(1)2", false},
		{modules.Route{Pick: true}, "US 01", false},
	} {
		filter, err := regexp2.Compile(tc.r.NodeFilter("p", prefix), 0)
		if err != nil {
			t.Fatal(err)
		}
		got, err := filter.MatchString(prefix + tc.name)
		if err != nil || got != tc.want {
			t.Errorf("%q: %v, %v; want %v", tc.name, got, err, tc.want)
		}
	}
}
