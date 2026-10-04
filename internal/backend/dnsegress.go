package backend

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/localhost-copilot/clashcube/internal/appdir"
	"github.com/localhost-copilot/clashcube/internal/mihomoapi"
)

// DNSEgress is where name lookups leave: the resolver that answers them
// and the address authoritative servers see the queries come from, which
// is the resolver's (or the proxy's), not this Mac's.
type DNSEgress struct {
	Via     string   `json:"via"`     // "mihomo", or "system" when the profile has no dns section
	Mode    string   `json:"mode"`    // fake-ip | redir-host; "" for the system resolver
	Servers []string `json:"servers"` // the profile's nameservers, as written
	IP      string   `json:"ip"`      // the egress address; "" when unknown
	Loc     string   `json:"loc"`     // the country of IP
	ECS     string   `json:"ecs"`     // the client subnet passed upstream, if any
}

// Akamai answers this TXT with the address the query reached it from.
const dnsWhoami = "whoami.ds.akahelp.net"

// runtimeDNS is the dns section the core runs with; enabled false when
// the profile leaves names to the system.
func runtimeDNS() (enabled bool, mode string, servers []string) {
	b, err := os.ReadFile(appdir.RuntimeConfig())
	if err != nil {
		return false, "", nil
	}
	var m struct {
		DNS struct {
			Enable       bool     `yaml:"enable"`
			EnhancedMode string   `yaml:"enhanced-mode"`
			Nameserver   []string `yaml:"nameserver"`
		} `yaml:"dns"`
	}
	if yaml.Unmarshal(b, &m) != nil || !m.DNS.Enable {
		return false, "", nil
	}
	// mihomo's default
	if m.DNS.EnhancedMode == "" {
		m.DNS.EnhancedMode = "redir-host"
	}
	return true, m.DNS.EnhancedMode, m.DNS.Nameserver
}

// DNSEgress asks the resolver the core uses, through the core, or the
// system's when the profile has none, where its queries come out.
func (b *Backend) DNSEgress() (DNSEgress, error) {
	c, err := b.Client()
	if err != nil {
		return DNSEgress{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	var e DNSEgress
	_, e.Mode, e.Servers = runtimeDNS()
	e.Via = "mihomo"
	txt, err := c.DNSLookup(ctx, dnsWhoami, "TXT")
	var ae *mihomoapi.APIError
	if errors.As(err, &ae) && strings.Contains(ae.Message, "disabled") {
		e = DNSEgress{Via: "system"}
		txt, err = net.DefaultResolver.LookupTXT(ctx, dnsWhoami)
	}
	if err != nil {
		return e, err
	}
	e.IP, e.ECS = parseWhoami(txt)
	e.Loc = country(e.IP)
	return e, nil
}

// parseWhoami reads Akamai's records: "ns" with the resolver's egress,
// "ecs" with the client subnet. The core writes a record as `"ns" "1.2.3.4"`;
// Go's resolver joins its strings into "ns1.2.3.4".
func parseWhoami(txt []string) (ns, ecs string) {
	for _, r := range txt {
		r = strings.NewReplacer(`"`, "", " ", "").Replace(r)
		if v, ok := strings.CutPrefix(r, "ecs"); ok {
			ecs = v
		} else if v, ok := strings.CutPrefix(r, "ns"); ok {
			ns = v
		}
	}
	return ns, ecs
}
