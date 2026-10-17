package runtimecfg

import "fmt"

// Run after every module: an applications module may replace the entire rule
// list, but managed private destinations still belong to their tailnet.
func tailscaleRoutes(config map[string]any) error {
	proxies, _ := config["proxies"].([]any)
	name := ""
	for _, raw := range proxies {
		p, ok := raw.(map[string]any)
		if !ok || p["type"] != "tailscale" || p["auto-route"] != true {
			continue
		}
		if name != "" {
			return fmt.Errorf("only one managed Tailscale connection can be enabled for a profile")
		}
		name, _ = p["name"].(string)
		if name == "" {
			return fmt.Errorf("managed Tailscale connection needs a name")
		}
	}
	if name == "" {
		return nil
	}
	rule := "TAILSCALE," + name + "," + name
	rules, _ := config["rules"].([]any)
	out := []any{rule}
	for _, r := range rules {
		if r != rule {
			out = append(out, r)
		}
	}
	config["rules"] = out
	// MagicDNS must go through the core even for profiles without DNS settings.
	dns, _ := config["dns"].(map[string]any)
	if dns == nil {
		dns = map[string]any{}
	}
	dns["enable"] = true
	if ns, _ := dns["nameserver"].([]any); len(ns) == 0 {
		dns["nameserver"] = defaultNameservers
	}
	config["dns"] = dns
	return nil
}
