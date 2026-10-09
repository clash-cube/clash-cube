package backend

import (
	"context"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

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
func pingMS(ctx context.Context, host string) float64 {
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
	return roundMS(f)
}
