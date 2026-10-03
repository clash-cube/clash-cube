package backend

import (
	"context"
	"errors"
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

// Connectivity measures all four at once.
func (b *Backend) Connectivity() (Connectivity, error) {
	var out Connectivity
	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	wg.Add(1)
	go func() {
		defer wg.Done()
		out.Gateway = gateway()
		if out.Gateway == "" {
			out.Router = -1
			return
		}
		out.Router = pingMS(ctx, out.Gateway)
	}()

	c, err := b.Client()
	if err != nil {
		wg.Wait()
		return out, err
	}
	testURL := settings.Load().TestURL

	wg.Add(3)
	go func() {
		defer wg.Done()
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
				return
			}
			out.DNS = ms(time.Since(start))
			return
		}
		out.DNSVia = "mihomo"
		if err != nil && !strings.Contains(err.Error(), "rcode 3") { // NXDOMAIN still answered
			out.DNS = -1
			return
		}
		out.DNS = ms(time.Since(start))
	}()
	go func() {
		defer wg.Done()
		d, err := c.Delay(ctx, "DIRECT", testURL, 5*time.Second)
		out.Internet = orFail(d, err)
	}()
	go func() {
		defer wg.Done()
		out.Via = b.mainGroup(ctx)
		if out.Via == "" {
			out.Proxy = -1
			return
		}
		d, err := c.Delay(ctx, out.Via, testURL, 5*time.Second)
		out.Proxy = orFail(d, err)
	}()
	wg.Wait()
	return out, nil
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
