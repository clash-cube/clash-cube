package backend

import (
	"cmp"
	"context"
	"errors"
	"net"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/localhost-copilot/clashcube/internal/mihomoapi"
	"github.com/localhost-copilot/clashcube/internal/modules"
	"github.com/localhost-copilot/clashcube/internal/runtimecfg"
)

// Globe is where the connections go now, by country, for the overview's
// globe: from this Mac's country, through the country a node's name gives,
// to the destination's.
type Globe struct {
	Origin   string           `json:"origin"`   // this Mac's country; "" while unknown
	OriginIP string           `json:"originIp"` // the public address it was found from
	Places   map[string]Place `json:"places"`   // the centre of every country named here
	Routes   []GlobeRoute     `json:"routes"`   // the busiest first
}

type Place struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// GlobeRoute is the connections to one country by one way: directly, or
// through nodes in one country.
type GlobeRoute struct {
	To     string      `json:"to"`
	Via    string      `json:"via"` // the nodes' country; "" for DIRECT, or nodes whose names don't say
	Direct bool        `json:"direct"`
	Conns  int         `json:"conns"`
	Up     int64       `json:"up"` // bytes a second
	Down   int64       `json:"down"`
	Total  int64       `json:"total"` // what its connections have moved
	Hosts  []GlobeHost `json:"hosts"` // the busiest, at most globeHosts
}

type GlobeHost struct {
	Host  string `json:"host"`
	Total int64  `json:"total"`
}

const (
	globeHosts = 5
	// a host's address is looked up again after this
	hostTTL = 10 * time.Minute
	// lookups under way at once; the rest wait for a later call
	hostLookups = 8
	maxHosts    = 4096
	// calls closer than this share a measure
	minSample = 800 * time.Millisecond
	// speeds are measured over about this long, as apps move data in bursts
	speedWindow = 3 * time.Second
)

// globeState is what Globe keeps between calls: the last byte counts, for
// speeds, the addresses of names the connections don't carry one for, and
// this Mac's country.
type globeState struct {
	mu       sync.Mutex
	snaps    []byteSnap
	rates    map[string][2]int64
	hosts    map[string]hostAddr
	pending  map[string]bool
	nodes    map[string]string
	origin   string
	originIP string
	originAt time.Time
	locating bool
}

// byteSnap is each connection's upload and download at a time.
type byteSnap struct {
	at   time.Time
	seen map[string][2]int64
}

type hostAddr struct {
	ip string // "" when the name didn't resolve
	at time.Time
}

// Globe groups the core's connections by the country they go to. Names
// without an address are resolved through the core in the background, so
// their connections show from a later call.
func (b *Backend) Globe() (Globe, error) {
	out := Globe{Places: map[string]Place{}, Routes: []GlobeRoute{}}
	c, err := b.Client()
	if err != nil {
		return out, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conns, err := c.Connections(ctx)
	if err != nil {
		return out, err
	}
	g := &b.globe
	g.mu.Lock()
	rates := g.sample(time.Now(), conns.Connections)
	out.Routes = globeRoutes(conns.Connections, rates, func(cn mihomoapi.Connection) string { return country(g.addrOf(c, cn)) }, g.nodeCountry)
	out.Origin, out.OriginIP = g.locate(b)
	g.mu.Unlock()
	for _, r := range out.Routes {
		for _, cc := range []string{r.To, r.Via} {
			if p, ok := countryCentres[cc]; ok {
				out.Places[cc] = Place{p[0], p[1]}
			}
		}
	}
	if p, ok := countryCentres[out.Origin]; ok {
		out.Places[out.Origin] = Place{p[0], p[1]}
	}
	return out, nil
}

// globeRoutes groups conns by destination and way; to gives a connection's
// destination country, via a node's.
func globeRoutes(conns []mihomoapi.Connection, rates map[string][2]int64, to func(mihomoapi.Connection) string, via func(string) string) []GlobeRoute {
	type acc struct {
		GlobeRoute
		hosts map[string]int64
	}
	byKey := map[string]*acc{}
	var order []*acc
	for _, c := range conns {
		if len(c.Chains) == 0 {
			continue
		}
		node := c.Chains[0]
		if node == "REJECT" || node == "REJECT-DROP" {
			continue
		}
		dest := to(c)
		if !hasCentre(dest) {
			continue
		}
		direct := node == "DIRECT"
		v := ""
		if !direct {
			if v = via(node); !hasCentre(v) {
				v = ""
			}
		}
		key := dest + "|" + v
		if direct {
			key += "|direct"
		}
		a := byKey[key]
		if a == nil {
			a = &acc{GlobeRoute: GlobeRoute{To: dest, Via: v, Direct: direct}, hosts: map[string]int64{}}
			byKey[key] = a
			order = append(order, a)
		}
		r := rates[c.ID]
		a.Conns++
		a.Up += r[0]
		a.Down += r[1]
		a.Total += c.Upload + c.Download
		md := c.Metadata
		if h := cmp.Or(md.Host, md.SniffHost, md.DestIP); h != "" {
			a.hosts[h] += c.Upload + c.Download
		}
	}
	out := make([]GlobeRoute, 0, len(order))
	for _, a := range order {
		a.Hosts = make([]GlobeHost, 0, len(a.hosts))
		for h, n := range a.hosts {
			a.Hosts = append(a.Hosts, GlobeHost{h, n})
		}
		slices.SortFunc(a.Hosts, func(x, y GlobeHost) int {
			return cmp.Or(cmp.Compare(y.Total, x.Total), strings.Compare(x.Host, y.Host))
		})
		a.Hosts = a.Hosts[:min(len(a.Hosts), globeHosts)]
		out = append(out, a.GlobeRoute)
	}
	slices.SortStableFunc(out, func(x, y GlobeRoute) int {
		return cmp.Or(cmp.Compare(y.Up+y.Down, x.Up+x.Down), cmp.Compare(y.Total, x.Total), cmp.Compare(y.Conns, x.Conns))
	})
	return out
}

// sample is each connection's speed over the last few seconds; after a
// pause (the page was hidden) it starts over, with no speeds. A window in
// the background calls less often, so the pause is long.
func (g *globeState) sample(now time.Time, conns []mihomoapi.Connection) map[string][2]int64 {
	if n := len(g.snaps); n > 0 {
		d := now.Sub(g.snaps[n-1].at)
		if d >= 0 && d < minSample {
			return g.rates
		}
		if d < 0 || d > 30*time.Second {
			g.snaps = nil
		}
	}
	// the base is the newest measure at least the window old
	for len(g.snaps) > 1 && now.Sub(g.snaps[1].at) >= speedWindow {
		g.snaps = g.snaps[1:]
	}
	seen := make(map[string][2]int64, len(conns))
	rates := map[string][2]int64{}
	for _, c := range conns {
		seen[c.ID] = [2]int64{c.Upload, c.Download}
	}
	if len(g.snaps) > 0 {
		base := g.snaps[0]
		secs := now.Sub(base.at).Seconds()
		for _, c := range conns {
			up, down := c.Upload, c.Download
			// a connection opened since the base moved all its bytes since
			if before, ok := base.seen[c.ID]; ok {
				up, down = max(up-before[0], 0), max(down-before[1], 0)
			} else if c.Start.Before(base.at) {
				continue
			}
			rates[c.ID] = [2]int64{int64(float64(up) / secs), int64(float64(down) / secs)}
		}
	}
	g.snaps, g.rates = append(g.snaps, byteSnap{now, seen}), rates
	return rates
}

// addrOf is the address a connection goes to: the one it carries, the one
// a direct one dialled, or its name's, looked up through the core.
func (g *globeState) addrOf(c *mihomoapi.Client, cn mihomoapi.Connection) string {
	md := cn.Metadata
	if md.DestIP != "" && !isFake(md.DestIP) {
		return md.DestIP
	}
	if len(cn.Chains) > 0 && cn.Chains[0] == "DIRECT" {
		if ip := bareIP(md.RemoteDest); ip != "" && !isFake(ip) {
			return ip
		}
	}
	host := strings.ToLower(strings.TrimSuffix(cmp.Or(md.Host, md.SniffHost), "."))
	if host == "" {
		return ""
	}
	h, ok := g.hosts[host]
	if (!ok || time.Since(h.at) > hostTTL) && !g.pending[host] && len(g.pending) < hostLookups {
		if g.pending == nil {
			g.pending = map[string]bool{}
		}
		g.pending[host] = true
		go g.resolveHost(c, host)
	}
	return h.ip
}

func (g *globeState) resolveHost(c *mihomoapi.Client, host string) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	ip := ""
	ans, err := c.DNSAnswers(ctx, host, "A")
	var ae *mihomoapi.APIError
	if errors.As(err, &ae) && strings.Contains(ae.Message, "disabled") {
		// the profile has no DNS: the core uses the system's
		if addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host); err == nil {
			for _, a := range addrs {
				ans = append(ans, mihomoapi.DNSAnswer{Type: 1, Data: a.IP.String()})
			}
		}
	}
	for _, a := range ans {
		if a.Type == 1 && !isFake(a.Data) {
			ip = a.Data
			break
		}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.pending, host)
	if g.hosts == nil || len(g.hosts) >= maxHosts {
		g.hosts = map[string]hostAddr{}
	}
	g.hosts[host] = hostAddr{ip, time.Now()}
}

// bareIP reads "ip" or "ip:port"; "" for anything else.
func bareIP(s string) string {
	if h, _, err := net.SplitHostPort(s); err == nil {
		s = h
	}
	if net.ParseIP(s) == nil {
		return ""
	}
	return s
}

func (g *globeState) nodeCountry(node string) string {
	if cc, ok := g.nodes[node]; ok {
		return cc
	}
	if g.nodes == nil || len(g.nodes) >= maxHosts {
		g.nodes = map[string]string{}
	}
	cc := nodeCountry(node)
	g.nodes[node] = cc
	return cc
}

// nodeCountry is the country a node's name gives: its flag, or a region
// routes know by name; "" when it gives none.
func nodeCountry(node string) string {
	name := runtimecfg.NodeLabel(node)
	rs := []rune(name)
	for i := 0; i+1 < len(rs); i++ {
		if regional(rs[i]) && regional(rs[i+1]) {
			return string([]rune{'A' + rs[i] - 0x1F1E6, 'A' + rs[i+1] - 0x1F1E6})
		}
	}
	for _, r := range modules.Regions {
		if r.Matches(name) {
			return strings.ToUpper(r.Key)
		}
	}
	return ""
}

func regional(r rune) bool { return r >= 0x1F1E6 && r <= 0x1F1FF }

func hasCentre(cc string) bool { _, ok := countryCentres[cc]; return ok }

// locate is this Mac's country and the public address of its direct route
// it comes from, looked up again in the background now and then.
func (g *globeState) locate(b *Backend) (string, string) {
	ttl := 10 * time.Minute
	if g.origin == "" {
		ttl = 30 * time.Second
	}
	if !g.locating && time.Since(g.originAt) > ttl {
		g.locating = true
		go func() {
			e, _ := b.DirectEgress()
			g.mu.Lock()
			defer g.mu.Unlock()
			// the broadband line's, as a router's proxy may move the other
			ip, cc := e.DomesticIP, e.DomesticLoc
			if cc == "" {
				ip, cc = e.IP, e.Loc
			}
			if cc = strings.ToUpper(cc); cc != "" && cc != "XX" {
				g.origin, g.originIP = cc, ip
			}
			g.originAt, g.locating = time.Now(), false
		}()
	}
	return g.origin, g.originIP
}
