package backend

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/metacubex/mihomo/component/geodata/memconservative"
	"github.com/metacubex/mihomo/component/geodata/router"
	P "github.com/metacubex/mihomo/constant/provider"
	"github.com/metacubex/mihomo/rules/provider"
	"go.yaml.in/yaml/v3"
	"google.golang.org/protobuf/proto"

	"github.com/localhost-copilot/clashcube/internal/appdir"
)

// RuleEntries is what a rule set, GeoSite list or GeoIP list holds, read
// from the files the core uses. Entries are those matching the query, at
// most the limit asked for.
type RuleEntries struct {
	Source   string   `json:"source"`   // the file, relative to the core's home; "inline" for one in the profile
	Behavior string   `json:"behavior"` // domain | ipcidr | classical, for rule sets
	Total    int      `json:"total"`
	Matched  int      `json:"matched"`
	Entries  []string `json:"entries"`
	// Note is why there is nothing to list: not fetched yet, or a database
	// that can't be listed. English, translated by the window.
	Note string `json:"note,omitempty"`
	// Not means the rule matches everything except these (GEOSITE,!cn).
	Not bool `json:"not,omitempty"`
}

// errNotFetched is a rule set or database the core hasn't downloaded yet.
var errNotFetched = errors.New("The core hasn't downloaded it yet")

// entryCache keeps the last lists read, keyed by kind, name and file
// version, so a search doesn't parse a large list on every keystroke.
type entryCache struct {
	mu   sync.Mutex
	keys []string
	vals map[string]RuleEntries
}

func (c *entryCache) get(key string, load func() (RuleEntries, error)) (RuleEntries, error) {
	c.mu.Lock()
	if v, ok := c.vals[key]; ok {
		c.mu.Unlock()
		return v, nil
	}
	c.mu.Unlock()
	v, err := load()
	if err != nil {
		return v, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.vals == nil {
		c.vals = map[string]RuleEntries{}
	}
	if _, ok := c.vals[key]; !ok {
		c.keys = append(c.keys, key)
		if len(c.keys) > 8 {
			delete(c.vals, c.keys[0])
			c.keys = c.keys[1:]
		}
	}
	c.vals[key] = v
	return v, nil
}

var ruleEntryCache entryCache

// RuleEntries lists a rule set (kind ruleset), a GeoSite list (geosite)
// or a GeoIP list (geoip) by the name the rule gives it.
func (b *Backend) RuleEntries(kind, name, query string, limit int) (RuleEntries, error) {
	var (
		all RuleEntries
		err error
	)
	switch kind {
	case "ruleset":
		all, err = ruleSetEntries(name)
	case "geosite", "geoip":
		all, err = geoEntries(kind, name)
	default:
		return RuleEntries{}, fmt.Errorf("unknown list kind %q", kind)
	}
	if errors.Is(err, errNotFetched) {
		return RuleEntries{Source: all.Source, Behavior: all.Behavior, Entries: []string{}, Note: err.Error()}, nil
	}
	if err != nil {
		return RuleEntries{}, err
	}
	return filterEntries(all, query, limit), nil
}

func filterEntries(all RuleEntries, query string, limit int) RuleEntries {
	out := all
	out.Total = len(all.Entries)
	out.Entries = []string{}
	q := strings.ToLower(strings.TrimSpace(query))
	for _, e := range all.Entries {
		if q != "" && !strings.Contains(strings.ToLower(e), q) {
			continue
		}
		out.Matched++
		if limit <= 0 || len(out.Entries) < limit {
			out.Entries = append(out.Entries, e)
		}
	}
	return out
}

// providerSchema is the part of a rule-providers entry that says where its
// rules are.
type providerSchema struct {
	Type     string   `yaml:"type"`
	Behavior string   `yaml:"behavior"`
	Format   string   `yaml:"format"`
	Path     string   `yaml:"path"`
	URL      string   `yaml:"url"`
	Payload  []string `yaml:"payload"`
}

func runtimeConfig() (map[string]any, error) {
	body, err := os.ReadFile(appdir.RuntimeConfig())
	if err != nil {
		return nil, err
	}
	var config map[string]any
	if err := yaml.Unmarshal(body, &config); err != nil {
		return nil, err
	}
	return config, nil
}

func ruleSetEntries(name string) (RuleEntries, error) {
	config, err := runtimeConfig()
	if err != nil {
		return RuleEntries{}, err
	}
	providers, _ := config["rule-providers"].(map[string]any)
	raw, ok := providers[name]
	if !ok {
		return RuleEntries{}, fmt.Errorf("no rule provider %q", name)
	}
	// through YAML again, so the schema decodes as the core reads it
	body, _ := yaml.Marshal(raw)
	var p providerSchema
	if err := yaml.Unmarshal(body, &p); err != nil {
		return RuleEntries{}, err
	}
	if p.Behavior == "" {
		p.Behavior = "classical"
	}
	if p.Type == "inline" {
		return RuleEntries{Source: "inline", Behavior: p.Behavior, Entries: p.Payload}, nil
	}
	path, err := providerPath(p)
	if err != nil {
		return RuleEntries{}, err
	}
	rel, _ := filepath.Rel(appdir.CoreHome(), path)
	head := RuleEntries{Source: rel, Behavior: p.Behavior}
	fi, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return head, errNotFetched
	}
	if err != nil {
		return head, err
	}
	key := fmt.Sprintf("ruleset\x00%s\x00%s\x00%d\x00%d", name, path, fi.Size(), fi.ModTime().UnixNano())
	return ruleEntryCache.get(key, func() (RuleEntries, error) {
		buf, err := os.ReadFile(path)
		if err != nil {
			return head, err
		}
		head.Entries, err = parseRuleSet(buf, p.Behavior, p.Format)
		return head, err
	})
}

// providerPath is the file the core keeps a provider in: its path, or for
// a remote one without a path, rules/<md5 of the URL> (constant/path.go).
// Like the core, only files inside its home.
func providerPath(p providerSchema) (string, error) {
	home := appdir.CoreHome()
	path := p.Path
	if path == "" {
		if p.Type != "http" {
			return "", errors.New("the rule provider has no path")
		}
		sum := md5.Sum([]byte(p.URL))
		path = filepath.Join("rules", hex.EncodeToString(sum[:]))
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(home, path)
	}
	path = filepath.Clean(path)
	if rel, err := filepath.Rel(home, path); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s is outside the core's home", p.Path)
	}
	return path, nil
}

// parseRuleSet reads a rule set's file as the core does, but keeps the
// entries as written. MRS, which is binary, goes through the core's own
// reader.
func parseRuleSet(buf []byte, behavior, format string) ([]string, error) {
	switch format {
	case "mrs":
		b, err := P.ParseBehavior(behavior)
		if err != nil {
			return nil, err
		}
		var out bytes.Buffer
		if err := provider.ConvertToMrs(buf, b, P.MrsRule, &out); err != nil {
			return nil, err
		}
		return lines(out.String(), false), nil
	case "text":
		return lines(string(buf), true), nil
	case "", "yaml":
		var v struct {
			Payload []string `yaml:"payload"`
			Rules   []string `yaml:"rules"`
		}
		if err := yaml.Unmarshal(buf, &v); err != nil {
			return nil, err
		}
		out := make([]string, 0, len(v.Payload)+len(v.Rules))
		for _, s := range append(v.Payload, v.Rules...) {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("unsupported format %q", format)
}

func lines(s string, comments bool) []string {
	out := []string{}
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || comments && (l[0] == '#' || strings.HasPrefix(l, "//")) {
			continue
		}
		out = append(out, l)
	}
	return out
}

// geoEntries lists a GeoSite or GeoIP code from the .dat database in the
// core's home. GeoSite codes may carry attributes (cn@ads) and be negated
// (!cn), as GEOSITE rules allow.
func geoEntries(kind, name string) (RuleEntries, error) {
	code, not := strings.CutPrefix(strings.TrimSpace(name), "!")
	parts := strings.Split(strings.ToLower(code), "@")
	code = strings.TrimSpace(parts[0])
	file := "GeoSite.dat"
	if kind == "geoip" {
		file = "GeoIP.dat"
		if config, err := runtimeConfig(); err == nil {
			if mode, _ := config["geodata-mode"].(bool); !mode {
				return RuleEntries{Source: "MMDB", Entries: []string{}, Note: "GeoIP comes from an MMDB database, which can't be listed"}, nil
			}
		}
	}
	path, fi := geoFile(file)
	head := RuleEntries{Source: filepath.Base(path), Not: not}
	if fi == nil {
		return head, errNotFetched
	}
	key := fmt.Sprintf("%s\x00%s\x00%s\x00%d\x00%d", kind, name, path, fi.Size(), fi.ModTime().UnixNano())
	return ruleEntryCache.get(key, func() (RuleEntries, error) {
		buf, err := memconservative.Decode(path, code)
		if err != nil {
			return head, fmt.Errorf("%s isn't in %s", code, filepath.Base(path))
		}
		if kind == "geoip" {
			var g router.GeoIP
			if err := proto.Unmarshal(buf, &g); err != nil {
				return head, err
			}
			head.Not = head.Not != g.ReverseMatch
			for _, c := range g.Cidr {
				if a, ok := netip.AddrFromSlice(c.Ip); ok {
					head.Entries = append(head.Entries, netip.PrefixFrom(a.Unmap(), int(c.Prefix)).String())
				}
			}
			return head, nil
		}
		var g router.GeoSite
		if err := proto.Unmarshal(buf, &g); err != nil {
			return head, err
		}
		attrs := parts[1:]
		for _, d := range g.Domain {
			if hasAttrs(d, attrs) {
				head.Entries = append(head.Entries, domainEntry(d))
			}
		}
		return head, nil
	})
}

// geoFile finds a database in the core's home, ignoring case as the core does.
func geoFile(name string) (string, os.FileInfo) {
	home := appdir.CoreHome()
	es, _ := os.ReadDir(home)
	for _, e := range es {
		if !e.IsDir() && strings.EqualFold(e.Name(), name) {
			if fi, err := e.Info(); err == nil {
				return filepath.Join(home, e.Name()), fi
			}
		}
	}
	return filepath.Join(home, name), nil
}

func hasAttrs(d *router.Domain, attrs []string) bool {
	for _, a := range attrs {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		found := false
		for _, have := range d.Attribute {
			if strings.EqualFold(have.GetKey(), a) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// domainEntry writes a GeoSite domain in the rule-set syntax mihomo's
// domain lists use: +.suffix, an exact name, keyword: and regexp:.
func domainEntry(d *router.Domain) string {
	switch d.Type {
	case router.Domain_Domain:
		return "+." + d.Value
	case router.Domain_Full:
		return d.Value
	case router.Domain_Regex:
		return "regexp:" + d.Value
	default:
		return "keyword:" + d.Value
	}
}
