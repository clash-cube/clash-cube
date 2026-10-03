package backend

import (
	"bufio"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/localhost-copilot/mihomobar/internal/mihomoapi"
	"github.com/localhost-copilot/mihomobar/internal/settings"
)

// Connectivity is the four latencies Surge's overview shows (docs/design.md
// §10.4): the router, DNS, the internet directly, and through the proxy.
// Each is in ms; 0 means not measured, -1 failed.
type Connectivity struct {
	Router   int      `json:"router"`
	DNS      int      `json:"dns"`
	Internet int      `json:"internet"`
	Proxy    int      `json:"proxy"`
	Gateway  string   `json:"gateway"`
	Via      string   `json:"via"`     // the policy the mode and rules sent the proxy test to
	Chain    []string `json:"chain"`   // from the node out to Via
	DNSVia   string   `json:"dnsVia"`  // "mihomo", or "system" when the profile has no dns section
	DNSMode  string   `json:"dnsMode"` // the core's enhanced-mode: fake-ip | redir-host
}

func ms(d time.Duration) int { return max(1, int(d.Milliseconds())) }

// ConnectivityItems are the keys ConnectivityItem takes.
var ConnectivityItems = []string{"router", "dns", "internet", "proxy"}

// Connectivity measures all four at once.
func (b *Backend) Connectivity() (Connectivity, error) {
	var out Connectivity
	var wg sync.WaitGroup
	var firstErr error
	var mu sync.Mutex
	ctx, cancel := context.WithTimeout(context.Background(), 11*time.Second)
	defer cancel()
	for _, key := range ConnectivityItems {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// each probe writes only its own fields
			if err := b.probe(ctx, key, &out); err != nil {
				mu.Lock()
				firstErr = cmp.Or(firstErr, err)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return out, firstErr
}

// ConnectivityItem measures one of ConnectivityItems, filling only its
// fields, so a caller can show each as soon as it is done.
func (b *Backend) ConnectivityItem(key string) (Connectivity, error) {
	var out Connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 11*time.Second)
	defer cancel()
	err := b.probe(ctx, key, &out)
	return out, err
}

func (b *Backend) probe(ctx context.Context, key string, out *Connectivity) error {
	// the egress lookups name countries from it; have it by then
	go geoReader()
	if key == "router" {
		out.Gateway = gateway()
		if out.Gateway == "" {
			out.Router = -1
			return nil
		}
		out.Router = pingMS(ctx, out.Gateway)
		return nil
	}

	c, err := b.Client()
	if err != nil {
		return err
	}
	testURL := settings.Load().TestURL

	switch key {
	case "dns":
		// a fresh name each time, so no cache answers it
		name := strconv.FormatInt(time.Now().UnixNano(), 36) + ".apple.com"
		start := time.Now()
		err := c.DNSQuery(ctx, name)
		var ae *mihomoapi.APIError
		if errors.As(err, &ae) && strings.Contains(ae.Message, "disabled") {
			// the profile leaves DNS to the system: time the system's
			out.DNSVia = "system"
			start = time.Now()
			if _, err := net.DefaultResolver.LookupHost(ctx, name); err != nil && !isNotFound(err) {
				out.DNS = -1
				return nil
			}
			out.DNS = ms(time.Since(start))
			return nil
		}
		out.DNSVia = "mihomo"
		_, out.DNSMode, _ = runtimeDNS()
		if err != nil && !strings.Contains(err.Error(), "rcode 3") { // NXDOMAIN still answered
			out.DNS = -1
			return nil
		}
		out.DNS = ms(time.Since(start))
	case "internet":
		d, err := c.Delay(ctx, "DIRECT", testURL, 5*time.Second)
		out.Internet = orFail(d, err)
	case "proxy":
		// timed as the core times its own, so the figure matches the
		// proxies page (and keeps the profile's unified-delay)
		_, out.Chain = throughCore(ctx, c, http.MethodHead, testURL)
		if len(out.Chain) == 0 {
			out.Proxy = -1
			return nil
		}
		out.Via = out.Chain[len(out.Chain)-1]
		d, err := c.Delay(ctx, out.Via, testURL, 5*time.Second)
		out.Proxy = orFail(d, err)
	default:
		return fmt.Errorf("unknown connectivity item %q", key)
	}
	return nil
}

func isNotFound(err error) bool {
	var de *net.DNSError
	return errors.As(err, &de) && de.IsNotFound
}

func orFail(d int, err error) int {
	if err != nil || d <= 0 {
		return -1
	}
	return d
}

// throughCore requests u through the mixed port, as an app would, so the
// mode and the rules pick the route and no group has to be guessed. It
// gives the body and the chain the core sent it along, from the node out
// to the policy the rule named, read off the core's tunnel while it is
// still open; nil when the request failed.
func throughCore(ctx context.Context, c *mihomoapi.Client, method, u string) (string, []string) {
	pu, err := url.Parse(u)
	if err != nil || pu.Hostname() == "" {
		return "", nil
	}
	target := pu.Host
	if pu.Port() == "" {
		port := "80"
		if pu.Scheme == "https" {
			port = "443"
		}
		target = net.JoinHostPort(pu.Hostname(), port)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// a CONNECT tunnel, even for http: the core keeps it as one connection
	// until we close it, where a plain proxied request is gone at once
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(proxyHost, strconv.Itoa(settings.Load().MixedPort)))
	if err != nil {
		return "", nil
	}
	defer conn.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodConnect})
	if err != nil || resp.StatusCode != http.StatusOK {
		return "", nil
	}
	// the core answers CONNECT before it dials: a request through the
	// tunnel waits for the route to be made
	var once atomic.Bool
	tr := &http.Transport{
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			if once.Swap(true) {
				return nil, errors.New("tunnel used")
			}
			return conn, nil
		},
		DisableCompression: true,
	}
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return "", nil
	}
	resp, err = tr.RoundTrip(req)
	if err != nil {
		return "", nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	resp.Body.Close()

	conns, err := c.Connections(ctx)
	if err != nil {
		return "", nil
	}
	port := strconv.Itoa(conn.LocalAddr().(*net.TCPAddr).Port)
	for _, cn := range conns.Connections {
		if cn.Metadata.SourcePort == port && len(cn.Chains) > 0 {
			return string(body), cn.Chains
		}
	}
	return "", nil
}

var gatewayRe = regexp.MustCompile(`gateway:\s*(\S+)`)

// gateway is the default route's next hop. Under TUN the default route is
// the tunnel, so the physical interface's gateway is looked up instead.
func gateway() string {
	out, err := exec.Command("/sbin/route", "-n", "get", "default").Output()
	if err == nil {
		if m := gatewayRe.FindSubmatch(out); m != nil && net.ParseIP(string(m[1])) != nil {
			if ip := net.ParseIP(string(m[1])); ip.To4() != nil && !strings.HasPrefix(ip.String(), "198.18.") {
				return ip.String()
			}
		}
	}
	// the DHCP router of en0
	out, err = exec.Command("/usr/sbin/ipconfig", "getoption", "en0", "router").Output()
	if err == nil {
		if ip := net.ParseIP(strings.TrimSpace(string(out))); ip != nil {
			return ip.String()
		}
	}
	return ""
}

var pingRe = regexp.MustCompile(`time=([\d.]+) ms`)

// pingMS sends one ICMP echo (ping needs no root on macOS).
func pingMS(ctx context.Context, host string) int {
	out, err := exec.CommandContext(ctx, "/sbin/ping", "-c", "1", "-t", "2", "-n", host).Output()
	if err != nil {
		return -1
	}
	m := pingRe.FindSubmatch(out)
	if m == nil {
		return -1
	}
	f, err := strconv.ParseFloat(string(m[1]), 64)
	if err != nil {
		return -1
	}
	return max(1, int(f+0.5))
}
