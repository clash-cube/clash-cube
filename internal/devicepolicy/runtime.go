package devicepolicy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

type Node struct {
	Target
	Reason string `json:"reason,omitempty"`
	raw    map[string]any
}

// Only concrete remote adapters can be pinned. Groups, rule adapters and
// dialer chains are intentionally excluded until their whole graph is proven.
func remoteNode(raw map[string]any) bool {
	kind, _ := raw["type"].(string)
	if raw["dialer-proxy"] != nil {
		return false
	}
	switch strings.ToLower(kind) {
	case "ss", "ssr", "vmess", "vless", "trojan", "hysteria", "hysteria2", "tuic", "socks5", "http", "anytls", "wireguard", "ssh", "mieru", "openconnect":
		return raw["server"] != nil
	}
	return false
}

func Catalog(config map[string]any, profile, coreHome string) []Node {
	out := []Node{}
	appendNodes := func(items []any, provider string) {
		for _, item := range items {
			raw, ok := item.(map[string]any)
			if !ok {
				continue
			}
			name, _ := raw["name"].(string)
			if name == "" || strings.HasPrefix(name, "CLASHCUBE-DEVICE-") {
				continue
			}
			kind, _ := raw["type"].(string)
			// Hash the whole concrete adapter, including credentials and transport
			// options, without exposing any of those fields in the public catalog.
			body, fingerprintErr := json.Marshal(raw)
			sum := sha256.Sum256(body)
			n := Node{Target: Target{Name: name, Profile: profile, Provider: provider, Type: kind, Fingerprint: hex.EncodeToString(sum[:])}, raw: raw}
			if !remoteNode(raw) {
				n.Reason = "Only fixed remote nodes without dialer chains are supported"
			}
			if fingerprintErr != nil {
				n.Reason = "Node configuration cannot be safely fingerprinted"
			}
			out = append(out, n)
		}
	}
	items, _ := config["proxies"].([]any)
	appendNodes(items, "")
	providers, _ := config["proxy-providers"].(map[string]any)
	for name, item := range providers {
		p, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if inline, ok := p["payload"].([]any); ok {
			appendNodes(inline, name)
		}
		path, _ := p["path"].(string)
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(coreHome, path)
		}
		rel, err := filepath.Rel(coreHome, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil || len(body) > 32<<20 {
			continue
		}
		var document struct {
			Proxies []any `yaml:"proxies"`
		}
		if yaml.Unmarshal(body, &document) == nil {
			appendNodes(document.Proxies, name)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func ReadRuntime(path string) (map[string]any, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var config map[string]any
	err = yaml.Unmarshal(body, &config)
	return config, err
}

func Match(nodes []Node, target Target) (Node, bool) {
	for _, n := range nodes {
		if n.Target == target && n.Reason == "" {
			return n, true
		}
	}
	return Node{}, false
}

// Overlay clones the selected adapter into a private, static proxy. Provider
// refresh cannot retarget it under the same display name. Missing identities
// retain a REJECT listener rather than a fallback to ordinary rule routing.
func Overlay(config map[string]any, c Config, profile, coreHome string) error {
	if len(c.Policies) == 0 {
		return nil
	}
	if err := ManagementPortsAvailable(config); err != nil {
		return err
	}
	nodes := Catalog(config, profile, coreHome)
	listeners, _ := config["listeners"].([]any)
	proxies, _ := config["proxies"].([]any)
	for _, item := range proxies {
		if p, ok := item.(map[string]any); ok {
			if name, _ := p["name"].(string); strings.HasPrefix(name, "CLASHCUBE-DEVICE-") {
				return fmt.Errorf("device proxy prefix is reserved")
			}
		}
	}
	claims := listenerPorts(config)
	for _, item := range listeners {
		if l, ok := item.(map[string]any); ok {
			if name, _ := l["name"].(string); strings.HasPrefix(name, "CLASHCUBE-DEVICE-") {
				return fmt.Errorf("device listener prefix is reserved")
			}
		}
	}
	for _, p := range c.Policies {
		for _, port := range []int{p.RedirPort(), p.DNSPort(), p.TunnelPort()} {
			for _, claim := range claims {
				if containsPort(claim, port) {
					return fmt.Errorf("port %d is reserved for device protection", port)
				}
			}
		}
		target := "REJECT"
		if node, ok := Match(nodes, p.Target); ok && !p.Paused {
			clone := map[string]any{}
			for key, value := range node.raw {
				clone[key] = value
			}
			clone["name"] = p.Inbound()
			proxies = append(proxies, clone)
			target = p.Inbound()
		}
		listeners = append(listeners, map[string]any{"name": p.Inbound(), "type": "redir", "listen": c.LAN.Address, "port": p.RedirPort(), "proxy": target},
			map[string]any{"name": p.Inbound() + "-DNS", "type": "tunnel", "listen": "127.0.0.1", "port": p.TunnelPort(), "network": []any{"tcp"}, "target": "1.1.1.1:53", "proxy": target})
	}
	config["proxies"], config["listeners"] = proxies, listeners
	return nil
}

// The firewall preserves these administration ports. They must never host a
// shared mihomo inbound, which would otherwise be a direct-routing escape.
func ManagementPortsAvailable(config map[string]any) error {
	for _, claim := range listenerPorts(config) {
		for _, port := range []int{22, 80, 443} {
			if containsPort(claim, port) {
				return fmt.Errorf("Ports 22, 80 and 443 are reserved for router management while device protection is enabled")
			}
		}
	}
	return nil
}

func listenerPorts(config map[string]any) []any {
	claims := []any{}
	for _, key := range []string{"mixed-port", "port", "socks-port", "redir-port", "tproxy-port"} {
		claims = append(claims, config[key])
	}
	if listeners, ok := config["listeners"].([]any); ok {
		for _, item := range listeners {
			if l, ok := item.(map[string]any); ok {
				claims = append(claims, l["port"])
			}
		}
	}
	if dns, ok := config["dns"].(map[string]any); ok {
		if listen, ok := dns["listen"].(string); ok {
			_, port, _ := net.SplitHostPort(listen)
			claims = append(claims, port)
		}
	}
	return claims
}

// mihomo's BaseOption accepts strings, lists and reversed ranges; its parser
// casts endpoints to uint16. Checking only YAML integers misses real binds.
func containsPort(value any, port int) bool {
	var text string
	switch v := value.(type) {
	case string:
		text = v
	case int:
		text = strconv.Itoa(v)
	case uint64:
		text = strconv.FormatUint(v, 10)
	case float64:
		text = strconv.FormatInt(int64(v), 10)
	default:
		return false
	}
	for _, part := range strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == '/' }) {
		ends := strings.Split(strings.TrimSpace(part), "-")
		if len(ends) > 2 {
			continue
		}
		first, err := strconv.ParseUint(strings.Trim(ends[0], "[ ]"), 10, 64)
		if err != nil {
			continue
		}
		lo, hi := int(uint16(first)), int(uint16(first))
		if len(ends) == 2 {
			last, err := strconv.ParseUint(strings.Trim(ends[1], "[ ]"), 10, 64)
			if err != nil {
				continue
			}
			hi = int(uint16(last))
		}
		if lo > hi {
			lo, hi = hi, lo
		}
		if port >= lo && port <= hi {
			return true
		}
	}
	return false
}

func Loaded(config map[string]any, p Policy) bool {
	items, _ := config["listeners"].([]any)
	for _, item := range items {
		if l, ok := item.(map[string]any); ok && l["name"] == p.Inbound() && l["proxy"] == p.Inbound() {
			return true
		}
	}
	return false
}
