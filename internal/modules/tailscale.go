package modules

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Tailscale is one persistent, browser-authenticated tailnet connection. Its
// identity is independent of display names and profiles; Routes are an explicit
// choice to send these destinations to the remote network, including offline.
type Tailscale struct {
	Identity   string   `json:"identity"`
	Hostname   string   `json:"hostname"`
	ControlURL string   `json:"controlURL,omitempty"`
	Routes     []string `json:"routes"`
}

const TailscalePrefix = "ClashCube Tailscale "

var tailscaleIdentity = regexp.MustCompile(`^[a-zA-Z0-9-]{8,64}$`)
var tailscaleHostname = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

func (t *Tailscale) ProxyName() string { return TailscalePrefix + t.Identity }

func (t *Tailscale) Check() error {
	if !tailscaleIdentity.MatchString(t.Identity) {
		return errors.New("Tailscale needs a stable identity ID")
	}
	if !tailscaleHostname.MatchString(t.Hostname) {
		return errors.New("Tailscale hostname must be a DNS label of up to 63 letters, digits or hyphens")
	}
	if t.ControlURL != "" {
		u, err := url.Parse(t.ControlURL)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("Tailscale control server must be an HTTPS URL without credentials, query or fragment")
		}
	}
	seen := map[netip.Prefix]bool{}
	for _, route := range t.Routes {
		p, err := netip.ParsePrefix(route)
		if err != nil || p != p.Masked() || p.Bits() == 0 || p.Addr().Is4In6() || !p.Addr().IsGlobalUnicast() || p.Addr().IsLoopback() {
			return fmt.Errorf("Tailscale subnet %q must be a canonical unicast CIDR, without a default route", route)
		}
		if seen[p] {
			return fmt.Errorf("duplicate Tailscale subnet %q", route)
		}
		seen[p] = true
	}
	return nil
}

func (t *Tailscale) Generate(_ string, config map[string]any) (string, error) {
	if err := t.Check(); err != nil {
		return "", err
	}
	name := t.ProxyName()
	if Policies(config)[name] {
		return "", fmt.Errorf("Tailscale connection %q is already present", name)
	}
	p := map[string]any{"name": name, "type": "tailscale", "hostname": t.Hostname,
		"state-dir": "tailscale/" + t.Identity, "ephemeral": false, "udp": true, "accept-routes": true, "auto-route": true, "route-prefixes": t.Routes}
	if t.ControlURL != "" {
		p["control-url"] = t.ControlURL
	}
	rules := []string{"TAILSCALE," + name + "," + name}
	b, err := yaml.Marshal(map[string]any{"append-proxies": []any{p}, "prepend-rules": rules})
	return string(b), err
}

func (t *Tailscale) Only(_, _ string) Kind {
	copy := *t
	copy.Routes = append([]string{}, t.Routes...)
	return &copy
}
func (*Tailscale) CopyProfile(_, _ string) bool { return false }
func (*Tailscale) ForgetProfile(_ string) bool  { return false }

// checkTailscales also understands converted YAML, so converting a form cannot
// accidentally permit two managed tailnets on an overlapping profile scope.
func checkTailscales(ms []Module) error {
	var scopes []Module
	for _, m := range ms {
		if !m.Enabled {
			continue
		}
		body := m.Body
		if k := m.Kind(); k != nil {
			body, _ = k.Generate("", nil)
		}
		v, err := Parse(body)
		if err != nil {
			return err
		}
		managed := 0
		for key, val := range v {
			if key != "append-proxies" && strings.Trim(key, "+!") != "proxies" {
				continue
			}
			list, _ := val.([]any)
			for _, p := range list {
				if p, ok := p.(map[string]any); ok && p["type"] == "tailscale" && p["auto-route"] == true {
					managed++
				}
			}
		}
		for i := 0; i < managed; i++ {
			for _, other := range scopes {
				if m.Profile == "" || other.Profile == "" || m.Profile == other.Profile {
					return errors.New("only one managed Tailscale connection can be enabled for a profile")
				}
			}
			scopes = append(scopes, m)
		}
	}
	return nil
}
