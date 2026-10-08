package modules

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Port is a module that serves one node or group on a SOCKS5 port of its
// own: what connects there leaves by it, whatever the rules and the mode
// say. The node is named per profile, since a name belongs to one
// subscription; a profile with none picked gets no port.
type Port struct {
	Port   int               `json:"port"`
	Listen string            `json:"listen,omitempty"` // a Listens address; "" is the first
	Target map[string]string `json:"target,omitempty"` // node or group, by profile ID
	UDP    bool              `json:"udp"`
	User   string            `json:"user,omitempty"`
	Pass   string            `json:"pass,omitempty"`
}

// Listens is the addresses a port can listen on: this Mac alone, or the LAN.
var Listens = []string{"127.0.0.1", "0.0.0.0"}

// PortPrefix starts the name of a port's listener; the port follows.
const PortPrefix = "ClashCube port "

// Addr is where the port listens.
func (p Port) Addr() string {
	if p.Listen == "" {
		return Listens[0]
	}
	return p.Listen
}

func (p *Port) Check() error {
	if p.Port < 1 || p.Port > 65535 {
		return fmt.Errorf("no port %d", p.Port)
	}
	ok := false
	for _, l := range Listens {
		ok = ok || p.Addr() == l
	}
	if !ok {
		return fmt.Errorf("can't listen on %q", p.Listen)
	}
	for id, name := range p.Target {
		if id == "" {
			return errors.New("a node without a profile")
		}
		if strings.TrimSpace(name) == "" || strings.ContainsAny(name, "\r\n") {
			return fmt.Errorf("bad node name %q", name)
		}
	}
	if (p.User == "") != (p.Pass == "") {
		return errors.New("a user needs a password, and a password a user")
	}
	for _, s := range []string{p.User, p.Pass} {
		if len(s) > 255 || strings.ContainsAny(s, "\r\n") {
			return errors.New("SOCKS5 takes a user and a password of up to 255 bytes, on one line")
		}
	}
	return nil
}

// Generate is the port's listener over the profile of that ID. A node the
// configuration doesn't declare (a provider's, or one that is gone) is
// reached through a hidden group that picks it by name, since a listener
// can only name declared ones; with no such node that group refuses.
func (p *Port) Generate(id string, config map[string]any) (string, error) {
	if err := p.Check(); err != nil {
		return "", err
	}
	target := p.Target[id]
	if target == "" {
		return "", nil
	}
	users := []any{}
	if p.User != "" {
		users = append(users, map[string]any{"username": p.User, "password": p.Pass})
	}
	port := strconv.Itoa(p.Port)
	l := map[string]any{"name": PortPrefix + port, "type": "socks", "listen": p.Addr(), "port": p.Port, "udp": p.UDP, "users": users, "proxy": target}
	m := map[string]any{"listeners+": []any{l}}
	if !Policies(config)[target] && !Builtin[target] {
		group := ChainPrefix + "port " + port
		l["proxy"] = group
		m["append-proxy-groups"] = []any{map[string]any{
			"name": group, "type": "select", "include-all": true, "filter": exactName(target), "empty-fallback": "REJECT", "hidden": true,
		}}
	}
	b, err := yaml.Marshal(m)
	return ReadableYAML(string(b)), err
}

// a listener has no rules, and may serve a group any module made
func (p *Port) trailing() {}

func (p *Port) Only(from, to string) Kind {
	c := *p
	c.Target = nil
	if name, ok := p.Target[from]; ok {
		c.Target = map[string]string{to: name}
	}
	return &c
}

func (p *Port) CopyProfile(from, to string) bool {
	name, ok := p.Target[from]
	if ok {
		p.Target[to] = name
	}
	return ok
}

func (p *Port) ForgetProfile(id string) bool {
	_, ok := p.Target[id]
	delete(p.Target, id)
	return ok
}

// checkPorts refuses two enabled ports on the same number that can be
// laid over the same profile: both global, or one global, or both one
// profile's.
func checkPorts(ms []Module) error {
	for i, a := range ms {
		for _, b := range ms[i+1:] {
			if a.Port == nil || b.Port == nil || !a.Enabled || !b.Enabled || a.Port.Port != b.Port.Port {
				continue
			}
			if a.Profile == "" || b.Profile == "" || a.Profile == b.Profile {
				return fmt.Errorf("%s and %s both use port %d", a.Name, b.Name, a.Port.Port)
			}
		}
	}
	return nil
}
