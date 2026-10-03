package backend

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/localhost-copilot/mihomobar/internal/mihomoapi"
	"github.com/localhost-copilot/mihomobar/internal/settings"
)

// Lookup is what the core makes of a name: how its resolver answers, and
// the rule and route a connection to it takes.
type Lookup struct {
	Host    string   `json:"host"`
	Port    int      `json:"port"`
	DNSMode string   `json:"dnsMode"` // the profile's enhanced-mode; "" with DNS off
	DNSVia  string   `json:"dnsVia"`  // mihomo, or system when the profile has no DNS
	A       []Record `json:"a"`
	AAAA    []Record `json:"aaaa"`
	CNAME   []string `json:"cname"`
	DNSErr  string   `json:"dnsError,omitempty"`
	// how a connection to host:port is routed
	Rule        string   `json:"rule"`
	RulePayload string   `json:"rulePayload"`
	Chain       []string `json:"chain"` // node first, as the core has it
	RemoteIP    string   `json:"remoteIp"`
	// RemoteIP is in the fake-ip range: another fake-ip resolver (a TUN
	// upstream, say) answered for the system
	RemoteFake bool   `json:"remoteFake"`
	RouteErr   string `json:"routeError,omitempty"`
}

// fake-ip's default pools
var fakeNets = []*net.IPNet{mustCIDR("198.18.0.0/15"), mustCIDR("fc00::/18")}

func mustCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return n
}

func isFake(s string) bool {
	ip := net.ParseIP(s)
	for _, n := range fakeNets {
		if ip != nil && n.Contains(ip) {
			return true
		}
	}
	return false
}

// Record is an address the resolver gave. Under fake-ip, apps are handed
// an address from the pool instead; these are the real ones the core
// connects to.
type Record struct {
	Data string `json:"data"`
	TTL  int    `json:"ttl"`
}

// LookupHost asks the core about host, a name or an address, optionally
// with a port (443 when none).
func (b *Backend) LookupHost(input string) (Lookup, error) {
	host, port, err := splitTarget(input)
	if err != nil {
		return Lookup{}, err
	}
	c, err := b.Client()
	if err != nil {
		return Lookup{}, err
	}
	out := Lookup{Host: host, Port: port, A: []Record{}, AAAA: []Record{}, CNAME: []string{}, Chain: []string{}}
	_, out.DNSMode, _ = runtimeDNS()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	if net.ParseIP(host) == nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out.DNSErr = b.resolve(ctx, c, host, &out)
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		r, err := routeOf(ctx, c, net.JoinHostPort(host, strconv.Itoa(port)))
		if err != nil {
			out.RouteErr = err.Error()
			return
		}
		out.Rule, out.RulePayload, out.Chain, out.RemoteIP = r.Rule, r.RulePayload, r.Chains, r.Metadata.RemoteDest
		if out.RemoteIP == "" {
			out.RemoteIP = r.Metadata.DestIP
		}
		out.RemoteFake = isFake(out.RemoteIP)
	}()
	wg.Wait()
	return out, nil
}

func (b *Backend) resolve(ctx context.Context, c *mihomoapi.Client, host string, out *Lookup) string {
	out.DNSVia = "mihomo"
	var errs []string
	for _, q := range []string{"A", "AAAA"} {
		ans, err := c.DNSAnswers(ctx, host, q)
		if err != nil {
			var ae *mihomoapi.APIError
			if errors.As(err, &ae) && strings.Contains(ae.Message, "disabled") {
				return systemResolve(ctx, host, out)
			}
			errs = append(errs, err.Error())
			continue
		}
		for _, a := range ans {
			switch a.Type {
			case 1, 28:
				r := Record{Data: a.Data, TTL: a.TTL}
				if a.Type == 1 {
					out.A = append(out.A, r)
				} else {
					out.AAAA = append(out.AAAA, r)
				}
			case 5:
				if q == "A" {
					out.CNAME = append(out.CNAME, strings.TrimSuffix(a.Data, "."))
				}
			}
		}
	}
	return strings.Join(errs, "; ")
}

// systemResolve asks the system's resolver, which the core uses when the
// profile has no DNS of its own.
func systemResolve(ctx context.Context, host string, out *Lookup) string {
	out.DNSVia = "system"
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		if isNotFound(err) {
			return ""
		}
		return err.Error()
	}
	for _, ip := range ips {
		r := Record{Data: ip.IP.String()}
		if ip.IP.To4() != nil {
			out.A = append(out.A, r)
		} else {
			out.AAAA = append(out.AAAA, r)
		}
	}
	if cname, err := net.DefaultResolver.LookupCNAME(ctx, host); err == nil {
		if cname = strings.TrimSuffix(cname, "."); cname != "" && cname != host {
			out.CNAME = append(out.CNAME, cname)
		}
	}
	return ""
}

// splitTarget reads "host", "host:port", a URL, or an IPv6 address.
func splitTarget(s string) (string, int, error) {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	host, port := s, 443
	if h, p, err := net.SplitHostPort(s); err == nil {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return "", 0, fmt.Errorf("bad port %q", p)
		}
		host, port = h, n
	}
	host = strings.TrimSuffix(strings.Trim(host, "[]"), ".")
	if host == "" || strings.ContainsAny(host, " \t,") {
		return "", 0, fmt.Errorf("not a host name or address: %q", s)
	}
	return strings.ToLower(host), port, nil
}

// routeOf opens a connection to target through the core, as an app would,
// and reads the rule and chain the core gave it (REJECT for a rejected
// one). Nothing is sent through it.
func routeOf(ctx context.Context, c *mihomoapi.Client, target string) (mihomoapi.Connection, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(proxyHost, strconv.Itoa(settings.Load().MixedPort)))
	if err != nil {
		return mihomoapi.Connection{}, err
	}
	defer conn.Close()
	port := strconv.Itoa(conn.LocalAddr().(*net.TCPAddr).Port)
	_ = conn.SetDeadline(time.Now().Add(6 * time.Second))
	fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodConnect})
	if err != nil {
		return mihomoapi.Connection{}, errors.New("the core closed the connection: rejected, or the target is unreachable")
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return mihomoapi.Connection{}, fmt.Errorf("the core answered %s", resp.Status)
	}
	// the core lists the connection once its route is dialled
	for i := 0; i < 30; i++ {
		conns, err := c.Connections(ctx)
		if err != nil {
			return mihomoapi.Connection{}, err
		}
		for _, cn := range conns.Connections {
			if cn.Metadata.SourcePort == port && cn.Metadata.SourceIP == proxyHost {
				_ = c.CloseConnection(ctx, cn.ID)
				return cn, nil
			}
		}
		select {
		case <-ctx.Done():
			return mihomoapi.Connection{}, ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
	return mihomoapi.Connection{}, errors.New("no route: rejected, or the target is unreachable")
}
