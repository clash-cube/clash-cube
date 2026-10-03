package backend

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Egress is where direct traffic leaves this Mac: the physical interface
// and the public address sites see. Upstream of this Mac (a router running
// a proxy, say) traffic may still be split by destination, so it is asked
// of two: a mainland site, which sees the broadband line, and Cloudflare,
// which sees where overseas traffic comes out. They differ when something
// upstream proxies.
type Egress struct {
	Interface   string `json:"interface"`   // e.g. en0
	Service     string `json:"service"`     // its network service, e.g. Wi-Fi
	DomesticIP  string `json:"domesticIp"`  // as a mainland site sees it; "" when unknown
	DomesticLoc string `json:"domesticLoc"` // the country of DomesticIP
	IP          string `json:"ip"`          // as Cloudflare sees it; "" when unknown
	Loc         string `json:"loc"`         // the country of IP
}

// ProxyEgress is where proxied traffic leaves: the address Cloudflare sees
// a request through the core come from, and the chain it took.
type ProxyEgress struct {
	IP    string   `json:"ip"`
	Loc   string   `json:"loc"`
	Chain []string `json:"chain"` // from the node out to the policy
}

// a name, unlike egressTrace, so the rules route it as they would a site
const proxyTrace = "https://www.cloudflare.com/cdn-cgi/trace"

const (
	// An IP literal needs no DNS, which under TUN's fake-ip would hand
	// out an address only the tunnel can reach.
	egressTrace = "https://1.1.1.1/cdn-cgi/trace"
	// answers with the bare address; its name is resolved by egressDNS
	domesticEcho = "https://ip.3322.net"
	egressDNS    = "223.5.5.5:53"
)

var ipv4Re = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)

// DirectEgress looks up the direct route's interface and public addresses.
// The requests, and the DNS for them, are bound to the physical interface,
// so neither TUN nor the rules route them through a proxy.
func (b *Backend) DirectEgress() (Egress, error) {
	var e Egress
	e.Interface = physicalInterface()
	if e.Interface == "" {
		return e, errors.New("no network interface")
	}
	e.Service = serviceOf(e.Interface)
	ifi, err := net.InterfaceByName(e.Interface)
	if err != nil {
		return e, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	get := boundGetter(ifi.Index)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		e.IP, e.Loc = parseTrace(get(ctx, egressTrace))
	}()
	go func() {
		defer wg.Done()
		e.DomesticIP = ipv4Re.FindString(get(ctx, domesticEcho))
		e.DomesticLoc = country(e.DomesticIP)
	}()
	wg.Wait()
	return e, nil
}

// ProxyEgress asks Cloudflare, through the core, where proxied traffic
// comes out.
func (b *Backend) ProxyEgress() (ProxyEgress, error) {
	c, err := b.Client()
	if err != nil {
		return ProxyEgress{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	body, chain := throughCore(ctx, c, http.MethodGet, proxyTrace)
	if chain == nil {
		return ProxyEgress{}, errors.New("the request through the core failed")
	}
	e := ProxyEgress{Chain: chain}
	e.IP, e.Loc = parseTrace(body)
	return e, nil
}

// parseTrace reads Cloudflare's trace: the address and its country, which
// is Cloudflare's own, or the local database's when it gives none.
func parseTrace(body string) (ip, loc string) {
	for _, l := range strings.Split(body, "\n") {
		k, v, _ := strings.Cut(l, "=")
		switch k {
		case "ip":
			ip = v
		case "loc":
			loc = v
		}
	}
	if loc == "" || loc == "XX" {
		loc = country(ip)
	}
	return ip, loc
}

// boundGetter fetches URLs over the interface, resolving names there too;
// a failure gives "".
func boundGetter(ifIndex int) func(ctx context.Context, u string) string {
	d := &net.Dialer{Control: func(_, _ string, rc syscall.RawConn) error {
		var serr error
		err := rc.Control(func(fd uintptr) {
			serr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, syscall.IP_BOUND_IF, ifIndex)
		})
		return errors.Join(err, serr)
	}}
	d.Resolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return d.DialContext(ctx, network, egressDNS)
	}}
	return func(ctx context.Context, u string) string {
		tr := &http.Transport{
			DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) { return d.DialContext(ctx, "tcp4", addr) },
		}
		defer tr.CloseIdleConnections()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return ""
		}
		resp, err := tr.RoundTrip(req)
		if err != nil {
			return ""
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return string(body)
	}
}

var interfaceRe = regexp.MustCompile(`interface:\s*(\S+)`)

// physicalInterface is the interface the router is reached through; under
// TUN the default route is the tunnel, but the gateway stays on the LAN.
func physicalInterface() string {
	gw := gateway()
	if gw == "" {
		return ""
	}
	out, err := exec.Command("/sbin/route", "-n", "get", gw).Output()
	if err != nil {
		return ""
	}
	if m := interfaceRe.FindSubmatch(out); m != nil && !strings.HasPrefix(string(m[1]), "utun") {
		return string(m[1])
	}
	return ""
}

var hardwarePortRe = regexp.MustCompile(`\(Hardware Port: ([^,]+), Device: ([^)]+)\)`)

// serviceOf is the network service on a device, as System Settings names
// it ("Wi-Fi"); "" when none is.
func serviceOf(device string) string {
	out, err := exec.Command("/usr/sbin/networksetup", "-listnetworkserviceorder").Output()
	if err != nil {
		return ""
	}
	for _, m := range hardwarePortRe.FindAllStringSubmatch(string(out), -1) {
		if m[2] == device {
			return m[1]
		}
	}
	return ""
}
