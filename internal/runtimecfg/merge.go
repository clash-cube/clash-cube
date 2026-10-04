package runtimecfg

import (
	"fmt"
	"strings"
)

// listOps are a module's top-level keys that add to the profile's lists
// rather than replace them, as Clash Verge and Clash Party write them.
var listOps = map[string]struct {
	key     string
	prepend bool
}{
	"prepend-rules":        {"rules", true},
	"append-rules":         {"rules", false},
	"prepend-proxies":      {"proxies", true},
	"append-proxies":       {"proxies", false},
	"prepend-proxy-groups": {"proxy-groups", true},
	"append-proxy-groups":  {"proxy-groups", false},
}

// Merge lays a module over m. Mappings merge key by key; anything else
// replaces what m has. A key may say otherwise: "+key" puts its list
// ahead of m's, "key+" after it, and "key!" replaces even a mapping. At
// the top, prepend-rules, append-proxies and the like add to the
// profile's rules, proxies and proxy groups.
func Merge(m, module map[string]any) error {
	for k, v := range module {
		if op, ok := listOps[k]; ok {
			if err := join(m, op.key, v, op.prepend); err != nil {
				return fmt.Errorf("%s: %w", k, err)
			}
			continue
		}
		if err := mergeKey(m, k, v); err != nil {
			return err
		}
	}
	return nil
}

func mergeKey(m map[string]any, k string, v any) error {
	switch {
	case strings.HasPrefix(k, "+") && len(k) > 1:
		if err := join(m, k[1:], v, true); err != nil {
			return fmt.Errorf("%s: %w", k, err)
		}
	case strings.HasSuffix(k, "+") && len(k) > 1:
		if err := join(m, k[:len(k)-1], v, false); err != nil {
			return fmt.Errorf("%s: %w", k, err)
		}
	case strings.HasSuffix(k, "!") && len(k) > 1:
		m[k[:len(k)-1]] = v
	default:
		sub, isMap := v.(map[string]any)
		have, hasMap := m[k].(map[string]any)
		if !isMap || !hasMap {
			m[k] = v
			return nil
		}
		for sk, sv := range sub {
			if err := mergeKey(have, sk, sv); err != nil {
				return fmt.Errorf("%s.%w", k, err)
			}
		}
	}
	return nil
}

// join puts list v ahead of m[k], or after it.
func join(m map[string]any, k string, v any, prepend bool) error {
	add, ok := v.([]any)
	if !ok {
		return fmt.Errorf("want a list, got %T", v)
	}
	have, _ := m[k].([]any)
	if _, isList := m[k].([]any); m[k] != nil && !isList {
		return fmt.Errorf("%s isn't a list", k)
	}
	out := make([]any, 0, len(add)+len(have))
	if prepend {
		out = append(append(out, add...), have...)
	} else {
		out = append(append(out, have...), add...)
	}
	m[k] = out
	return nil
}
