package backend

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/localhost-copilot/mihomobar/internal/mihomoapi"
	"github.com/localhost-copilot/mihomobar/internal/settings"
)

// Connectivity is the four latencies Surge's overview shows (docs/design.md
// §10.4): the router, DNS, the internet directly, and through the proxy.
// Each is in ms; 0 means not measured, -1 failed.
type Connectivity struct {
	Router   int    `json:"router"`
	DNS      int    `json:"dns"`
	Internet int    `json:"internet"`
	Proxy    int    `json:"proxy"`
	Gateway  string `json:"gateway"`
	Via      string `json:"via"`    // the group the proxy latency went through
	DNSVia   string `json:"dnsVia"` // "mihomo", or "system" when the profile has no dns section
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
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
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
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	err := b.probe(ctx, key, &out)
	return out, err
}

func (b *Backend) probe(ctx context.Context, key string, out *Connectivity) error {
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
		if err != nil && !strings.Contains(err.Error(), "rcode 3") { // NXDOMAIN still answered
			out.DNS = -1
			return nil
		}
		out.DNS = ms(time.Since(start))
	case "internet":
		d, err := c.Delay(ctx, "DIRECT", testURL, 5*time.Second)
		out.Internet = orFail(d, err)
	case "proxy":
		out.Via = b.mainGroup(ctx)
		if out.Via == "" {
			out.Proxy = -1
			return nil
		}
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

// mainGroup is the group most traffic goes through: GLOBAL in global mode,
// else the first group the profile lists.
func (b *Backend) mainGroup(ctx context.Context) string {
	c, err := b.Client()
	if err != nil {
		return ""
	}
	all, err := c.Proxies(ctx)
	if err != nil {
		return ""
	}
	if settings.Load().Mode == "global" {
		return "GLOBAL"
	}
	for _, name := range all["GLOBAL"].All {
		if p, ok := all[name]; ok && len(p.All) > 0 && !p.Hidden {
			return name
		}
	}
	return ""
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
