package runtimecfg

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/dlclark/regexp2"
	"github.com/localhost-copilot/clashcube/internal/modules"
	"go.yaml.in/yaml/v3"
)

// Generated nodes are private to service routes, including when another group
// uses include-all. Names are stable across reloads so selections survive.
const ChainPrefix = "ClashCube chain/"

// NodeLabel hides the private namespace while keeping original names intact.
func NodeLabel(name string) string {
	if rest, ok := strings.CutPrefix(name, ChainPrefix); ok {
		_, rest, _ = strings.Cut(rest, "/")
		if node, ok := strings.CutPrefix(rest, "nodes/"); ok {
			return node
		}
	}
	return name
}

func copyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func exactName(name string) string {
	return "^" + strings.ReplaceAll(regexp2.Escape(name), "`", `\x60`) + "$"
}

func upstreamBody(id string, r modules.Route, config map[string]any) (string, error) {
	if err := r.Check(); err != nil {
		return "", err
	}
	front := r.Upstream[id]
	have := policies(config)
	group := r.Group(func(n string) bool { return have[n] || builtin[n] })
	prefix := ChainPrefix + group + "/"
	nodePrefix := prefix + "nodes/"
	frontGroup := prefix + "upstream"
	fg := map[string]any{"name": frontGroup, "type": "select", "include-all": true, "filter": exactName(front), "empty-fallback": "REJECT", "hidden": true}
	var clones, names []any
	nodes, _ := config["proxies"].([]any)
	for _, item := range nodes {
		node, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := node["name"].(string)
		if name == front {
			fg["proxies"] = []string{front}
		}
		if name == front || strings.HasPrefix(name, ChainPrefix) || !r.Takes(id, name) {
			continue
		}
		clone := copyMap(node)
		clone["name"], clone["dialer-proxy"] = nodePrefix+name, frontGroup
		clones = append(clones, clone)
		names = append(names, nodePrefix+name)
	}
	providers, _ := config["proxy-providers"].(map[string]any)
	keys := make([]string, 0, len(providers))
	for key := range providers {
		if !strings.HasPrefix(key, ChainPrefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	added := map[string]any{}
	var use []string
	for _, key := range keys {
		original, ok := providers[key].(map[string]any)
		if !ok {
			continue
		}
		clone := copyMap(original)
		override, _ := original["override"].(map[string]any)
		override = copyMap(override)
		// Run after existing renames/overrides; the UI selects their effective names.
		exprs, _ := override["override-expr"].([]any)
		p, _ := json.Marshal(nodePrefix)
		f, _ := json.Marshal(frontGroup)
		override["override-expr"] = append(append([]any{}, exprs...), fmt.Sprintf(`.name = %s + .name`, p), fmt.Sprintf(`.["dialer-proxy"] = %s`, f))
		clone["override"] = override
		if clone["type"] == "http" {
			clone["path"] = fmt.Sprintf("providers/clashcube-%x.yaml", sha256.Sum256([]byte(prefix+key)))
		}
		added[prefix+"providers/"+key] = clone
		use = append(use, prefix+"providers/"+key)
	}
	// Reuse the service's rules, but scope its group to private nodes/providers.
	plain := r
	plain.Upstream = nil
	body, err := plain.Body(id, func(n string) bool { return have[n] || builtin[n] }, nil)
	if err != nil {
		return "", err
	}
	m, err := modules.Parse(body)
	if err != nil {
		return "", err
	}
	g := m["append-proxy-groups"].([]any)[0].(map[string]any)
	delete(g, "include-all")
	delete(g, "filter")
	if len(names) == 0 && len(use) == 0 {
		names = []any{"REJECT"}
	}
	g["proxies"], g["use"] = names, use
	g["filter"] = r.NodeFilter(id, nodePrefix)
	g["exclude-filter"] = exactName(nodePrefix + front)
	m["append-proxy-groups"] = []any{fg, g}
	m["append-proxies"], m["proxy-providers"] = clones, added
	b, err := yaml.Marshal(m)
	return modules.ReadableYAML(string(b)), err
}

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
