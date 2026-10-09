// Package modules keeps the user's modules: YAML snippets laid over the
// profiles, as Surge's modules are. Like the user's rules they live apart
// from the profiles, which are replaced on every update, and runtimecfg
// merges the enabled ones in order: the global ones over every profile,
// then a profile's own over it alone.
package modules

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"go.yaml.in/yaml/v3"

	"github.com/localhost-copilot/clashcube/internal/appdir"
)

type Module struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Body    string `json:"body"` // YAML, a mapping
	// the ID of the profile it is laid over alone; empty, over every one
	Profile string `json:"profile,omitempty"`
	// a service sent through a policy: the body is made from it, again
	// for each profile (runtimecfg), so its group's name never clashes
	Route *Route `json:"route,omitempty"`
	// a node or group served on a SOCKS5 port of its own
	Port *Port `json:"port,omitempty"`
	// for YAML taken from a kind's form, the kind it was: going back to the
	// form restores it, dropping the YAML
	Was *Module `json:"was,omitempty"`
}

// Owned is the keys the app sets itself: a module's are overridden (the
// controller's, so a module can't open another way into the core).
var Owned = []string{
	"mixed-port", "allow-lan", "ipv6", "mode", "log-level", "find-process-mode", "secret",
	"external-controller", "external-controller-unix", "external-controller-pipe", "external-controller-tls",
	"external-ui", "external-ui-url", "external-ui-name",
}

// Parse reads a module's body: a mapping, or nothing.
func Parse(body string) (map[string]any, error) {
	var m map[string]any
	if err := yaml.Unmarshal([]byte(body), &m); err != nil {
		var te *yaml.TypeError
		if errors.As(err, &te) {
			return nil, errors.New("a module is a mapping of keys, like the top of a profile")
		}
		return nil, err
	}
	return m, nil
}

// Check refuses a module without a name or with a body that isn't YAML.
func (m Module) Check() error {
	if strings.TrimSpace(m.Name) == "" {
		return errors.New("a module needs a name")
	}
	if m.kinds() > 1 {
		return fmt.Errorf("%s: a module is made from one kind", strings.TrimSpace(m.Name))
	}
	if k := m.Kind(); k != nil {
		if m.Was != nil {
			return fmt.Errorf("%s: only YAML is taken from a form", strings.TrimSpace(m.Name))
		}
		if err := k.Check(); err != nil {
			return fmt.Errorf("%s: %w", strings.TrimSpace(m.Name), err)
		}
		return nil
	}
	if m.Was != nil && (m.Was.kinds() != 1 || m.Was.Was != nil) {
		return fmt.Errorf("%s: a module's form is one kind", strings.TrimSpace(m.Name))
	}
	if _, err := Parse(m.Body); err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(m.Name), err)
	}
	return nil
}

// Keys is the top-level keys the module sets, as written ("+rules",
// "dns"), sorted; the app's own are left out.
func (m Module) Keys() []string {
	v, _ := Parse(m.Body)
	owned := map[string]bool{}
	for _, k := range Owned {
		owned[k] = true
	}
	var out []string
	for k := range v {
		if !owned[strings.Trim(k, "+!")] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// For is the enabled modules of ms laid over profile id, in the order
// they merge: the global ones, then the profile's own, so its rules,
// put first, go ahead of the global ones'.
func For(id string, ms []Module) []Module {
	var out []Module
	for _, own := range []bool{false, true} {
		for _, m := range ms {
			if m.Enabled && (m.Profile != "") == own && (!own || m.Profile == id) {
				out = append(out, m)
			}
		}
	}
	return out
}

var mu sync.Mutex

func path() string { return filepath.Join(appdir.Root(), "modules.json") }

// List is the modules, merged first to last.
func List() []Module {
	mu.Lock()
	defer mu.Unlock()
	ms := []Module{}
	if b, err := os.ReadFile(path()); err == nil {
		_ = json.Unmarshal(b, &ms)
	}
	return ms
}

// Save replaces the modules; one without an ID is given one.
func Save(ms []Module) error {
	seen := map[string]bool{}
	for i := range ms {
		if err := ms[i].Check(); err != nil {
			return err
		}
		ms[i].Name = strings.TrimSpace(ms[i].Name)
		if k := ms[i].Kind(); k != nil {
			ms[i].Body, _ = k.Generate("", nil)
		}
		if w := ms[i].Was; w != nil {
			was := Module{}.withKind(w.Kind())
			ms[i].Was = &was
		}
		if ms[i].ID == "" || seen[ms[i].ID] {
			ms[i].ID = newID()
		}
		seen[ms[i].ID] = true
	}
	if err := checkPorts(ms); err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	b, err := json.MarshalIndent(ms, "", "  ")
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

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
