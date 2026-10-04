// Package userrules keeps the rules the user adds in the app (Surge's "add
// rule for this host"). They live apart from the profiles, which are never
// changed in place and are replaced on every update, and runtimecfg puts
// them ahead of the profile's own rules.
package userrules

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/localhost-copilot/clashferry/internal/appdir"
)

// Rule is one rule, as mihomo writes it: TYPE,payload,policy.
type Rule struct {
	Type    string `json:"type"`
	Payload string `json:"payload"`
	Policy  string `json:"policy"`
}

func (r Rule) String() string { return r.Type + "," + r.Payload + "," + r.Policy }

// Types is the rule types the app offers; each takes one payload.
var Types = []string{
	"DOMAIN", "DOMAIN-SUFFIX", "DOMAIN-KEYWORD", "IP-CIDR", "IP-CIDR6",
	"SRC-IP-CIDR", "DST-PORT", "PROCESS-NAME", "PROCESS-PATH", "PROCESS-PATH-REGEX", "GEOIP", "GEOSITE",
}

// Check refuses a rule that isn't one plain TYPE,payload,policy line: a
// comma or a line break would smuggle in another field or rule.
func (r Rule) Check() error {
	known := false
	for _, t := range Types {
		known = known || t == r.Type
	}
	if !known {
		return fmt.Errorf("unknown rule type %q", r.Type)
	}
	for _, v := range []string{r.Payload, r.Policy} {
		if strings.TrimSpace(v) == "" {
			return errors.New("a rule needs a value and a policy")
		}
		if strings.ContainsAny(v, ",\r\n") {
			return errors.New("a rule's value and policy can't contain commas or line breaks")
		}
	}
	return nil
}

var mu sync.Mutex

func path() string { return filepath.Join(appdir.Root(), "rules.json") }

// List is the rules, first matched first.
func List() []Rule {
	mu.Lock()
	defer mu.Unlock()
	return load()
}

func load() []Rule {
	rs := []Rule{}
	if b, err := os.ReadFile(path()); err == nil {
		_ = json.Unmarshal(b, &rs)
	}
	return rs
}

// Save replaces the rules.
func Save(rs []Rule) error {
	for _, r := range rs {
		if err := r.Check(); err != nil {
			return err
		}
	}
	mu.Lock()
	defer mu.Unlock()
	b, err := json.MarshalIndent(rs, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(appdir.Root(), 0o755); err != nil {
		return err
	}
	tmp := path() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path())
}

// Added is rs with r first; a rule for the same type and payload is
// replaced, so adding one again changes its policy.
func Added(rs []Rule, r Rule) []Rule {
	out := []Rule{r}
	for _, o := range rs {
		if o.Type != r.Type || o.Payload != r.Payload {
			out = append(out, o)
		}
	}
	return out
}
