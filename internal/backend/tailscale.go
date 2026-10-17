package backend

import (
	"context"
	"net"
	"net/netip"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/localhost-copilot/clashcube/internal/mihomoapi"
	"github.com/localhost-copilot/clashcube/internal/modules"
)

func (b *Backend) TailscaleLogin(name string) error {
	c, err := b.Client()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return c.TailscaleLogin(ctx, name)
}
func (b *Backend) TailscaleLogout(name string) error {
	c, err := b.Client()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return c.TailscaleLogout(ctx, name)
}
func (b *Backend) TailscalePing(name, ip string) (mihomoapi.TailscalePing, error) {
	c, err := b.Client()
	if err != nil {
		return mihomoapi.TailscalePing{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return c.TailscalePing(ctx, name, ip)
}

// Local routes help explain overlapping LAN/subnet destinations before the
// user selects a remote subnet. This does not change the OS routing table.
func (*Backend) TailscaleLocalNetworks() []string {
	addrs, _ := net.InterfaceAddrs()
	seen := map[string]bool{}
	for _, addr := range addrs {
		if p, err := netip.ParsePrefix(addr.String()); err == nil && p.Addr().IsPrivate() {
			seen[p.Masked().String()] = true
		}
	}
	out := []string{}
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

var notHostname = regexp.MustCompile(`[^a-z0-9-]+`)

// TailscaleHostname is this computer's name as a DNS label, so each machine
// on one tailnet starts out under a name of its own.
func (*Backend) TailscaleHostname() string {
	name, _ := os.Hostname()
	name, _, _ = strings.Cut(strings.ToLower(name), ".")
	name = strings.Trim(notHostname.ReplaceAllString(name, "-"), "-")
	if len(name) > 63 {
		name = strings.TrimRight(name[:63], "-")
	}
	if name == "" {
		return "clashcube"
	}
	return name
}

// tailscaleReconnects reports whether ms changes how an enabled Tailscale
// connects. A running core keeps one session per identity and refuses a
// reload with other settings, so such a change needs a restart.
func tailscaleReconnects(before, ms []modules.Module) bool {
	was := map[string]modules.Tailscale{}
	for _, m := range before {
		if m.Enabled && m.Tailscale != nil {
			was[m.Tailscale.Identity] = *m.Tailscale
		}
	}
	for _, m := range ms {
		if !m.Enabled || m.Tailscale == nil {
			continue
		}
		if old, ok := was[m.Tailscale.Identity]; ok && (old.Hostname != m.Tailscale.Hostname || old.ControlURL != m.Tailscale.ControlURL) {
			return true
		}
	}
	return false
}
