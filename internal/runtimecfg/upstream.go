package runtimecfg

import (
	"strings"

	"github.com/dlclark/regexp2"
	"github.com/localhost-copilot/clashcube/internal/modules"
)

// Generated nodes are private to service routes, including when another group
// uses include-all. Names are stable across reloads so selections survive.
const ChainPrefix = modules.ChainPrefix

// NodeLabel hides the private namespace while keeping original names intact.
func NodeLabel(name string) string { return modules.NodeLabel(name) }

func hideChainNodes(config map[string]any) {
	groups, _ := config["proxy-groups"].([]any)
	hasChain := false
	for _, item := range groups {
		if g, ok := item.(map[string]any); ok {
			name, _ := g["name"].(string)
			hasChain = hasChain || strings.HasPrefix(name, ChainPrefix)
		}
	}
	if !hasChain {
		return
	}
	for _, item := range groups {
		g, ok := item.(map[string]any)
		if !ok || (g["include-all"] != true && g["include-all-proxies"] != true && g["include-all-providers"] != true) {
			continue
		}
		exclude, _ := g["exclude-filter"].(string)
		if exclude != "" {
			exclude += "`"
		}
		g["exclude-filter"] = exclude + "^" + regexp2.Escape(ChainPrefix)
	}
}
