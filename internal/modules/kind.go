package modules

// Kind is a module made from choices on the page rather than written as
// YAML: a Route, a Port. Its body is generated again for each profile when
// the configuration is built, against what that profile and the modules
// before it have, so names it refers to are the profile's own.
//
// Adding a kind: a type implementing Kind, a pointer field on Module for
// it (so the bindings type it), and a case in Module.Kind.
type Kind interface {
	Check() error
	// Generate is the module's body over the profile of that ID, given the
	// configuration built so far (nil when there is none, for a preview).
	Generate(profile string, config map[string]any) (string, error)
	// Only is a copy for the module's copy laid over profile to alone,
	// keeping just the choices made for profile from, as to's.
	Only(from, to string) Kind
	// CopyProfile gives profile to the choices made for from, and
	// ForgetProfile drops id's; both say whether anything changed.
	CopyProfile(from, to string) bool
	ForgetProfile(id string) bool
}

// Grouper is a kind that makes a group of its own; GroupIn is its name in
// a configuration with the names has, "" for none.
type Grouper interface {
	GroupIn(has func(string) bool) string
}

// Trailing says whether k adds nothing whose place in the order matters
// (no rules), so it is laid over the profile after every other module and
// can name what any of them made, such as a route's group.
func Trailing(k Kind) bool {
	_, ok := k.(interface{ trailing() })
	return ok
}

// Ordered is ms in the order they are laid over a profile: as given, but
// trailing kinds last.
func Ordered(ms []Module) []Module {
	out := make([]Module, 0, len(ms))
	for _, last := range []bool{false, true} {
		for _, m := range ms {
			if k := m.Kind(); (k != nil && Trailing(k)) == last {
				out = append(out, m)
			}
		}
	}
	return out
}

// Kind is the kind the module is made from, nil for one written as YAML.
func (m Module) Kind() Kind {
	switch {
	case m.Route != nil:
		return m.Route
	case m.Port != nil:
		return m.Port
	}
	return nil
}

// withKind is m made from k instead.
func (m Module) withKind(k Kind) Module {
	m.Route, m.Port = nil, nil
	switch k := k.(type) {
	case *Route:
		m.Route = k
	case *Port:
		m.Port = k
	}
	return m
}

// kinds counts how many kinds m is made from; more than one is refused.
func (m Module) kinds() int {
	n := 0
	for _, set := range []bool{m.Route != nil, m.Port != nil} {
		if set {
			n++
		}
	}
	return n
}

// CopyProfile gives a copy of profile from what from has: copies of its
// own modules, and the choices the global ones made for it.
func CopyProfile(from, to string) error {
	ms := List()
	var copies []Module
	changed := false
	for _, m := range ms {
		k := m.Kind()
		if m.Profile == from {
			c := m
			c.ID, c.Profile = "", to
			if k != nil {
				c = c.withKind(k.Only(from, to))
			}
			copies = append(copies, c)
			continue
		}
		if k != nil && m.Profile == "" && k.CopyProfile(from, to) {
			changed = true
		}
	}
	if !changed && len(copies) == 0 {
		return nil
	}
	return Save(append(ms, copies...))
}

// ForgetProfile drops a profile that is gone: its own modules, and the
// choices the global ones made for it.
func ForgetProfile(id string) error {
	ms := List()
	kept := ms[:0]
	changed := false
	for _, m := range ms {
		if m.Profile == id {
			changed = true
			continue
		}
		if k := m.Kind(); k != nil && k.ForgetProfile(id) {
			changed = true
		}
		kept = append(kept, m)
	}
	if !changed {
		return nil
	}
	return Save(kept)
}

// Policies is the names of config's proxies and groups.
func Policies(config map[string]any) map[string]bool {
	have := map[string]bool{}
	for _, k := range []string{"proxies", "proxy-groups"} {
		list, _ := config[k].([]any)
		for _, p := range list {
			if p, ok := p.(map[string]any); ok {
				if name, ok := p["name"].(string); ok {
					have[name] = true
				}
			}
		}
	}
	return have
}

// Builtin is the policies every profile has.
var Builtin = map[string]bool{"DIRECT": true, "REJECT": true, "REJECT-DROP": true, "PASS": true, "COMPATIBLE": true}

// declared is the names of config's top-level proxies, which, unlike a
// provider's, can be named directly.
func declared(config map[string]any) map[string]bool {
	out := map[string]bool{}
	nodes, _ := config["proxies"].([]any)
	for _, node := range nodes {
		if node, ok := node.(map[string]any); ok {
			if name, ok := node["name"].(string); ok {
				out[name] = true
			}
		}
	}
	return out
}
