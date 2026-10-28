package router

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/localhost-copilot/clashcube/internal/appdir"
	"github.com/localhost-copilot/clashcube/internal/backend"
	"github.com/localhost-copilot/clashcube/internal/devicepolicy"
	"github.com/localhost-copilot/clashcube/internal/runtimecfg"
	"github.com/localhost-copilot/clashcube/internal/settings"
)

type Status struct {
	Enabled   bool         `json:"enabled"`
	Active    bool         `json:"active"`
	Interface string       `json:"interface"`
	Error     string       `json:"error,omitempty"`
	Access    AccessPolicy `json:"access"`
}

// Manager builds validated router configuration. Firewall ownership remains
// in the core service; exiting this management session changes no network state.
type Manager struct {
	b        *backend.Backend
	path     string
	enabled  atomic.Bool
	opMu     sync.Mutex
	guard    FirewallControl
	discover func() (lan, error)
}

func New(b *backend.Backend, home string, guard FirewallControl) (*Manager, error) {
	m := &Manager{b: b, path: filepath.Join(home, "router.json"), guard: guard, discover: discoverLAN}
	p, err := readPreference(m.path)
	if err != nil {
		return nil, err
	}
	m.enabled.Store(p.Enabled)
	b.RuntimeOverlay = &runtimecfg.Overlay{Source: "router", Apply: m.overlay}
	b.AfterRuntimeReload = m.expireDeviceSessions
	return m, nil
}

func (m *Manager) save(on bool) error {
	p, err := readPreference(m.path)
	if err != nil {
		return err
	}
	p.Enabled = on
	if err := savePreference(m.path, p); err != nil {
		return err
	}
	m.enabled.Store(on)
	return nil
}

func (m *Manager) Get() (Status, error) {
	status, err := m.guard.Get()
	if err != nil {
		return status, err
	}
	p, err := readPreference(m.path)
	status.Access = p.Access
	return status, err
}

func (m *Manager) Reconcile() (Status, error) { return m.guard.Reconcile() }

func (m *Manager) waitReady() error {
	deadline := time.Now().Add(5 * time.Second)
	for {
		l, err := m.discover()
		if err == nil {
			c, e := m.b.Client()
			err = e
			if err == nil {
				err = ready(c, l)
			}
		}
		if err == nil || time.Now().After(deadline) {
			return err
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (m *Manager) SetEnabled(on bool) (Status, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	before := m.enabled.Load()
	if on && m.b.State().Core != "running" {
		return Status{}, errors.New("Start the core before enabling the LAN proxy")
	}
	if status, err := m.guard.Pause(); err != nil {
		return status, err
	}
	// Resume also releases the bounded pause after an early persistence error.
	defer m.guard.Resume()
	err := m.save(on)
	if err == nil {
		err = m.b.Reload()
	}
	if err == nil && on {
		err = m.waitReady()
	}
	var status Status
	if err == nil {
		status, err = m.guard.Resume()
		if err == nil && on && !status.Active {
			err = errors.New("LAN proxy did not become active")
		}
	}
	if err != nil {
		_, pauseErr := m.guard.Pause()
		saveErr := m.save(before)
		reloadErr := m.b.Reload()
		var resumeErr error
		status, resumeErr = m.guard.Resume()
		err = errors.Join(err, pauseErr, saveErr, reloadErr, resumeErr)
	}
	return status, err
}

func (m *Manager) overlay(config map[string]any) error {
	if err := m.overlayLAN(config); err != nil {
		return err
	}
	c, err := devicepolicy.Load(filepath.Dir(m.path))
	if err != nil {
		return err
	}
	return devicepolicy.Overlay(config, c, settings.Load().Profile, appdir.CoreHome())
}

func (m *Manager) overlayLAN(config map[string]any) error {
	allowLAN, _ := config["allow-lan"].(bool)
	if !m.enabled.Load() && !allowLAN {
		return nil
	}
	l, err := m.discover()
	if err != nil {
		return err
	}
	if allowLAN {
		// Keep loopback available for backend downloads and probes, and open
		// explicit proxy ports only on LAN rather than every router interface.
		config["bind-address"] = "127.0.0.1"
		listeners, _ := config["listeners"].([]any)
		for _, inbound := range []struct{ key, kind string }{{"mixed-port", "mixed"}, {"port", "http"}, {"socks-port", "socks"}} {
			if port, ok := config[inbound.key].(int); ok && port > 0 {
				// Preserve IN-NAME rules and global authentication/source filters.
				// Omitting users inherits mihomo's default authentication store.
				name := "DEFAULT-" + strings.ToUpper(inbound.kind)
				for _, item := range listeners {
					if existing, ok := item.(map[string]any); ok && existing["name"] == name {
						return fmt.Errorf("listener name %s is reserved for LAN proxy access", name)
					}
				}
				listener := map[string]any{"name": name, "type": inbound.kind, "listen": l.Address, "port": port}
				if inbound.kind != "http" {
					listener["udp"] = true
				}
				listeners = append(listeners, listener)
			}
		}
		config["listeners"] = listeners
	}
	if !m.enabled.Load() {
		return nil
	}
	for _, key := range []string{"mixed-port", "port", "socks-port", "redir-port", "tproxy-port"} {
		if p, ok := config[key].(int); ok && (p == redirectPort || p == dnsPort) {
			return fmt.Errorf("port %d is reserved for the LAN proxy", p)
		}
	}
	listeners, _ := config["listeners"].([]any)
	for _, item := range listeners {
		listener, _ := item.(map[string]any)
		if p, ok := listener["port"].(int); ok && (p == redirectPort || p == dnsPort) {
			return fmt.Errorf("port %d is reserved for the LAN proxy", p)
		}
	}
	config["listeners"] = append(listeners, map[string]any{"name": "ClashCube LAN", "type": "redir", "listen": l.Address, "port": redirectPort})
	dns, _ := config["dns"].(map[string]any)
	if dns == nil {
		dns = map[string]any{}
	}
	dns["enable"] = true
	dns["listen"] = net.JoinHostPort(l.Address, strconv.Itoa(dnsPort))
	// Non-DNS UDP is not intercepted; fake IPs would be unreachable for it.
	dns["enhanced-mode"] = "redir-host"
	dns["ipv6"] = false
	if ns, ok := dns["nameserver"].([]any); !ok || len(ns) == 0 {
		dns["nameserver"] = []any{"https://dns.alidns.com/dns-query", "https://doh.pub/dns-query"}
	}
	// Bootstrap proxy/upstream hostnames without asking dnsmasq, whose public
	// queries now return to this core. Preserve profile-supplied bootstrap DNS.
	if ns, ok := dns["default-nameserver"].([]any); !ok || len(ns) == 0 {
		dns["default-nameserver"] = []any{"223.5.5.5", "1.1.1.1"}
	}
	if err := checkDNSLoops(dns, l.Address); err != nil {
		return err
	}
	config["dns"] = dns
	return nil
}

func checkDNSLoops(dns map[string]any, address string) error {
	var check func(any) error
	check = func(value any) error {
		switch v := value.(type) {
		case []any:
			for _, entry := range v {
				if err := check(entry); err != nil {
					return err
				}
			}
		case map[string]any:
			for _, entry := range v {
				if err := check(entry); err != nil {
					return err
				}
			}
		case string:
			if v == "system" || strings.HasPrefix(v, "system://") || v == "dhcp://lan" {
				return errors.New("Router DNS upstream must not use the router's own DNS resolver")
			}
			target := v
			if !strings.Contains(target, "://") {
				target = "udp://" + target
			}
			u, err := url.Parse(target)
			if err != nil {
				return nil
			} // mihomo supplies the full syntax validation
			port := u.Port()
			if port == "" && (u.Scheme == "udp" || u.Scheme == "tcp") {
				port = "53"
			}
			host := u.Hostname()
			if (host == address || host == "127.0.0.1" || host == "localhost" || host == "::1" || host == "0.0.0.0") && (port == "53" || port == "1053") {
				return errors.New("Router DNS upstream would loop through dnsmasq or the core DNS listener")
			}
		}
		return nil
	}
	for _, key := range []string{"nameserver", "fallback", "default-nameserver", "proxy-server-nameserver", "direct-nameserver", "nameserver-policy"} {
		if err := check(dns[key]); err != nil {
			return err
		}
	}
	return nil
}
