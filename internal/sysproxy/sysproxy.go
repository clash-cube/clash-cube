//go:build darwin

// Package sysproxy sets macOS's system proxy on every enabled network
// service, through networksetup (which an admin user may run without root).
package sysproxy

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func run(args ...string) (string, error) {
	out, err := exec.Command("/usr/sbin/networksetup", args...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("networksetup %s: %v: %s", args[0], err, bytes.TrimSpace(out))
	}
	return string(out), nil
}

// Services is the enabled network services.
func Services() ([]string, error) {
	out, err := run("-listallnetworkservices")
	if err != nil {
		return nil, err
	}
	var svcs []string
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l == "" || strings.HasPrefix(l, "An asterisk") || strings.HasPrefix(l, "*") {
			continue
		}
		svcs = append(svcs, l)
	}
	return svcs, nil
}

// Set points HTTP, HTTPS and SOCKS at host:port on every service.
func Set(host string, port int, bypass []string) error {
	svcs, err := Services()
	if err != nil {
		return err
	}
	p := strconv.Itoa(port)
	var errs []string
	for _, s := range svcs {
		for _, kind := range []string{"webproxy", "securewebproxy", "socksfirewallproxy"} {
			if _, err := run("-set"+kind, s, host, p); err != nil {
				errs = append(errs, err.Error())
			}
		}
		if len(bypass) > 0 {
			if _, err := run(append([]string{"-setproxybypassdomains", s}, bypass...)...); err != nil {
				errs = append(errs, err.Error())
			}
		}
	}
	if len(errs) > 0 && len(errs) == len(svcs)*4 {
		return fmt.Errorf("%s", errs[0])
	}
	return nil
}

// Clear turns the three proxies off on every service.
func Clear() error {
	svcs, err := Services()
	if err != nil {
		return err
	}
	for _, s := range svcs {
		for _, kind := range []string{"webproxy", "securewebproxy", "socksfirewallproxy"} {
			_, _ = run("-set"+kind+"state", s, "off")
		}
	}
	return nil
}

// State is one service's HTTP proxy.
type State struct {
	Enabled bool
	Host    string
	Port    int
}

func get(kind, svc string) (State, error) {
	out, err := run("-get"+kind, svc)
	if err != nil {
		return State{}, err
	}
	var st State
	for _, l := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(l, ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch strings.TrimSpace(k) {
		case "Enabled":
			st.Enabled = v == "Yes"
		case "Server":
			st.Host = v
		case "Port":
			st.Port, _ = strconv.Atoi(v)
		}
	}
	return st, nil
}

// PointsAt says whether any service's HTTP proxy is on and at host:port.
func PointsAt(host string, port int) bool {
	svcs, err := Services()
	if err != nil {
		return false
	}
	for _, s := range svcs {
		if st, err := get("webproxy", s); err == nil && st.Enabled && st.Host == host && st.Port == port {
			return true
		}
	}
	return false
}

// Effective says whether the proxy macOS applies now, the primary service's
// as scutil reports it, is HTTP at host:port. Unlike PointsAt it takes one
// command, so it can be asked often.
func Effective(host string, port int) bool {
	out, err := exec.Command("/usr/sbin/scutil", "--proxy").Output()
	if err != nil {
		return false
	}
	return parseEffective(string(out), host, port)
}

func parseEffective(out, host string, port int) bool {
	kv := map[string]string{}
	for _, l := range strings.Split(out, "\n") {
		if k, v, ok := strings.Cut(l, " : "); ok {
			kv[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return kv["HTTPEnable"] == "1" && kv["HTTPProxy"] == host && kv["HTTPPort"] == strconv.Itoa(port)
}
