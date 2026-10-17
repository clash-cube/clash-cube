package runtimecfg

import (
	"github.com/localhost-copilot/clashcube/internal/modules"
	"github.com/localhost-copilot/clashcube/internal/settings"
	"go.yaml.in/yaml/v3"
	"testing"
)

func TestTailscaleSurvivesRuleReplacement(t *testing.T) {
	node := &modules.Tailscale{Identity: "identity-001", Hostname: "clashcube", Routes: []string{"192.168.50.0/24"}}
	mods := []modules.Module{{Name: "tailnet", Enabled: true, Tailscale: node}, {Name: "Replace rules", Enabled: true, Body: "rules: [MATCH,DIRECT]"}}
	profile := []byte("proxy-groups: [{name: automatic, type: url-test, include-all: true, exclude-filter: '^old$'}]")
	body, err := Build("p", profile, settings.Defaults(), Controller{}, nil, mods)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := yaml.Unmarshal(body, &cfg); err != nil {
		t.Fatal(err)
	}
	rules := cfg["rules"].([]any)
	if rules[0] != "TAILSCALE,"+node.ProxyName()+","+node.ProxyName() {
		t.Fatalf("private rule lost to catch-all: %v", rules)
	}
	if cfg["dns"].(map[string]any)["enable"] != true {
		t.Fatal("MagicDNS unavailable without profile DNS")
	}
	proxies := cfg["proxies"].([]any)
	p := proxies[0].(map[string]any)
	if p["route-prefixes"].([]any)[0] != "192.168.50.0/24" {
		t.Fatal("lost selected subnet")
	}
	mods = append(mods, modules.Module{Name: "second", Enabled: true, Tailscale: &modules.Tailscale{Identity: "identity-002", Hostname: "another"}})
	if _, err := Build("p", profile, settings.Defaults(), Controller{}, nil, mods); err == nil {
		t.Fatal("accepted two active tailnets")
	}
}
