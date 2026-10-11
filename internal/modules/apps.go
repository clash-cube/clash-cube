package modules

import (
	"errors"
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Apps is a module that proxies the programs it names and nothing else,
// as Proxifier does: their connections leave by the node or group picked
// for the profile, and every other connection goes direct. It takes the
// place of the profile's rules and every other module's, so it is laid
// over after them; the user's own rules still go first. A profile with no
// node picked is left as it is.
type Apps struct {
	Apps   []App             `json:"apps"`
	Target map[string]string `json:"target,omitempty"` // node or group, by profile ID
}

// App is a program as a process rule names it.
type App struct {
	Type    string `json:"type"` // one of AppTypes
	Payload string `json:"payload"`
}

// AppTypes is the rule types an App can be.
var AppTypes = []string{"PROCESS-NAME", "PROCESS-PATH", "PROCESS-PATH-REGEX"}

// AppsGroup is the hidden group the programs leave by when their node is
// one a rule can't name (a provider's, or one that is gone).
const AppsGroup = ChainPrefix + "apps"

func (a *Apps) Check() error {
	if len(a.Apps) == 0 {
		return errors.New("choose at least one program")
	}
	for _, app := range a.Apps {
		known := false
		for _, t := range AppTypes {
			known = known || app.Type == t
		}
		if !known {
			return fmt.Errorf("no program rule %q", app.Type)
		}
		// a comma or a line break would smuggle in another field or rule
		if strings.TrimSpace(app.Payload) == "" || strings.ContainsAny(app.Payload, ",\r\n") {
			return fmt.Errorf("bad program %q", app.Payload)
		}
	}
	for id, name := range a.Target {
		if id == "" {
			return errors.New("a node without a profile")
		}
		if strings.TrimSpace(name) == "" || strings.ContainsAny(name, ",\r\n") {
			return fmt.Errorf("bad node name %q", name)
		}
	}
	return nil
}

// Generate replaces the rules over the profile of that ID: the programs
// to their node, then everything else direct.
func (a *Apps) Generate(id string, config map[string]any) (string, error) {
	if err := a.Check(); err != nil {
		return "", err
	}
	target := a.Target[id]
	if target == "" {
		return "", nil
	}
	m := map[string]any{}
	policy := target
	if !Policies(config)[target] && !Builtin[target] {
		policy = AppsGroup
		m["append-proxy-groups"] = []any{map[string]any{
			"name": policy, "type": "select", "include-all": true, "filter": exactName(target), "empty-fallback": "REJECT", "hidden": true,
		}}
	}
	rules := make([]string, 0, len(a.Apps)+1)
	for _, app := range a.Apps {
		rules = append(rules, app.Type+","+app.Payload+","+policy)
	}
	m["rules!"] = append(rules, "MATCH,DIRECT")
	b, err := yaml.Marshal(m)
	return ReadableYAML(string(b)), err
}

// replacing the rules, it goes after every module that adds some, and may
// send the programs to a group any of them made
func (a *Apps) trailing() {}

func (a *Apps) Only(from, to string) Kind {
	c := *a
	c.Apps = append([]App{}, a.Apps...)
	c.Target = nil
	if name, ok := a.Target[from]; ok {
		c.Target = map[string]string{to: name}
	}
	return &c
}

func (a *Apps) CopyProfile(from, to string) bool {
	name, ok := a.Target[from]
	if ok {
		a.Target[to] = name
	}
	return ok
}

func (a *Apps) ForgetProfile(id string) bool {
	_, ok := a.Target[id]
	delete(a.Target, id)
	return ok
}

// checkApps refuses two enabled program modules that can be laid over the
// same profile: the later would throw away the earlier's rules.
func checkApps(ms []Module) error {
	for i, a := range ms {
		for _, b := range ms[i+1:] {
			if a.Apps == nil || b.Apps == nil || !a.Enabled || !b.Enabled {
				continue
			}
			if a.Profile == "" || b.Profile == "" || a.Profile == b.Profile {
				return fmt.Errorf("%s and %s both choose the only programs to proxy; turn one off", a.Name, b.Name)
			}
		}
	}
	return nil
}
