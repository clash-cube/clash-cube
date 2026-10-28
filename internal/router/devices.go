package router

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"path/filepath"
	"strconv"
	"time"

	"github.com/localhost-copilot/clashcube/internal/appdir"
	"github.com/localhost-copilot/clashcube/internal/devicepolicy"
	"github.com/localhost-copilot/clashcube/internal/settings"
)

type DeviceStatus struct {
	devicepolicy.Policy
	Protection   string `json:"protection"`
	Connectivity string `json:"connectivity"`
}

type DevicesView struct {
	Revision string                `json:"revision"`
	Devices  []devicepolicy.Device `json:"devices"`
	Policies []DeviceStatus        `json:"policies"`
	Nodes    []devicepolicy.Node   `json:"nodes"`
	Error    string                `json:"error,omitempty"`
}

type DeviceRequest struct {
	Reserve  bool                `json:"reserve"`
	Revision string              `json:"revision"`
	ID       string              `json:"id"`
	Name     string              `json:"name"`
	MAC      string              `json:"mac"`
	IP       string              `json:"ip"`
	Target   devicepolicy.Target `json:"target"`
}

func (m *Manager) deviceCatalog() ([]devicepolicy.Node, map[string]any, error) {
	config, err := devicepolicy.ReadRuntime(appdir.RuntimeConfig())
	if err != nil {
		return []devicepolicy.Node{}, nil, err
	}
	return devicepolicy.Catalog(config, settings.Load().Profile, appdir.CoreHome()), config, nil
}

func (m *Manager) Devices() (DevicesView, error) {
	c, err := devicepolicy.Load(filepath.Dir(m.path))
	if err != nil {
		return DevicesView{}, err
	}
	nodes, runtime, runtimeErr := m.deviceCatalog()
	v := DevicesView{Revision: c.Revision, Devices: devicepolicy.Discover(), Policies: []DeviceStatus{}, Nodes: nodes}
	if l, err := m.discover(); err == nil {
		if prefix, err := netip.ParsePrefix(l.Prefix); err == nil {
			devices := []devicepolicy.Device{}
			for _, d := range v.Devices {
				if ip, err := netip.ParseAddr(d.IP); err == nil && prefix.Contains(ip) {
					devices = append(devices, d)
				}
			}
			v.Devices = devices
		}
	}
	guardErr := devicepolicy.Verified(c)
	if len(c.Policies) > 0 && guardErr == nil {
		guardErr = devicepolicy.CheckCapabilities()
	}
	if guardErr != nil && len(c.Policies) > 0 {
		v.Error = guardErr.Error()
	}
	core := m.b.State().Core == "running"
	for _, p := range c.Policies {
		s := DeviceStatus{Policy: p, Protection: "unknown", Connectivity: "blocked"}
		if guardErr == nil {
			s.Protection = "protected"
		}
		switch {
		case identityChanged(v.Devices, p):
			s.Connectivity = "identity-changed"
		case p.Pending:
			s.Connectivity = "pending"
		case p.Paused:
			s.Connectivity = "paused"
		case !core:
			s.Connectivity = "core-stopped"
		case runtimeErr != nil:
			s.Connectivity = "unknown"
		case !devicepolicy.Loaded(runtime, p):
			s.Connectivity = "target-missing"
		default:
			s.Connectivity = "configured"
		}
		v.Policies = append(v.Policies, s)
	}
	return v, nil
}

func identityChanged(devices []devicepolicy.Device, p devicepolicy.Policy) bool {
	for _, d := range devices {
		if d.MAC == p.MAC && d.CurrentIP != "" && d.CurrentIP != p.IP {
			return true
		}
	}
	return false
}

func (m *Manager) deviceConfig(revision string) (devicepolicy.Config, error) {
	c, err := devicepolicy.Load(filepath.Dir(m.path))
	if err != nil {
		return c, err
	}
	if c.Revision != revision {
		return c, errors.New("Device policies changed; refresh before saving")
	}
	return c, nil
}

func (m *Manager) closeDeviceFlows(ip, mac string) error {
	if err := devicepolicy.Evict(ip, mac); err != nil {
		return err
	}
	if m.b.State().Core != "running" {
		return nil
	}
	c, err := m.b.Client()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connections, err := c.Connections(ctx)
	if err != nil {
		return err
	}
	for _, flow := range connections.Connections {
		if flow.Metadata.SourceIP == ip {
			if err := c.CloseConnection(ctx, flow.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// Existing proxied streams retain their original adapter across a mihomo
// reload. Expire them when a profile/module change could remove a fixed node.
func (m *Manager) expireDeviceSessions() error {
	config, err := devicepolicy.Load(filepath.Dir(m.path))
	if err != nil {
		return err
	}
	if len(config.Policies) == 0 {
		return nil
	}
	c, err := m.b.Client()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	flows, err := c.Connections(ctx)
	if err != nil {
		return err
	}
	for _, flow := range flows.Connections {
		for _, p := range config.Policies {
			if flow.Metadata.SourceIP == p.IP || flow.Metadata.InboundName == p.Inbound()+"-DNS" {
				if err := c.CloseConnection(ctx, flow.ID); err != nil {
					return err
				}
				break
			}
		}
	}
	return nil
}

// commitDevice first installs a closed gate, then reloads the fixed listener.
// A refused reload leaves Pending persisted, including across a router reboot.
func (m *Manager) commitDevice(c devicepolicy.Config, id string, reserve bool) (DevicesView, error) {
	if err := devicepolicy.CheckCapabilities(); err != nil {
		return DevicesView{}, err
	}
	if err := devicepolicy.CheckRules(c); err != nil {
		return DevicesView{}, err
	}
	home := filepath.Dir(m.path)
	c.Revision = devicepolicy.NewID()
	for i := range c.Policies {
		if c.Policies[i].ID == id {
			c.Policies[i].Pending = true
		}
	}
	if err := devicepolicy.PersistIntent(c); err != nil {
		return DevicesView{}, err
	}
	if err := devicepolicy.Save(home, c); err != nil {
		return DevicesView{}, err
	}
	if err := devicepolicy.Apply(home, c); err != nil {
		return DevicesView{}, err
	}
	p, _ := c.Find(id)
	if err := m.closeDeviceFlows(p.IP, p.MAC); err != nil {
		return DevicesView{}, err
	}
	if reserve {
		// Reuse an existing matching reservation. Never overwrite another
		// static section or silently reassign a previously reserved address.
		found := false
		for _, d := range devicepolicy.Discover() {
			if d.Reserved && d.MAC == p.MAC {
				if d.IP != p.IP {
					return DevicesView{}, errors.New("This device already has a different DHCP reservation")
				}
				found = true
			}
		}
		if !found {
			body, _ := json.Marshal(map[string]any{"config": "dhcp", "type": "host", "name": "clashcube_" + p.ID, "values": map[string]any{"mac": p.MAC, "ip": p.IP}})
			if _, err := run("ubus", "call", "uci", "add", string(body)); err != nil {
				return DevicesView{}, errors.New("Device remains blocked: could not reserve its DHCP address")
			}
			if _, err := run("ubus", "call", "uci", "commit", `{"config":"dhcp"}`); err != nil {
				return DevicesView{}, err
			}
			if _, err := run("/etc/init.d/dnsmasq", "reload"); err != nil {
				return DevicesView{}, err
			}
		}
	}
	if err := m.b.Reload(); err != nil {
		return DevicesView{}, errors.New("Device remains blocked: core rejected the new configuration")
	}
	if m.b.State().Core == "running" {
		// The supervisor starts relays independently of the replaceable web app.
		deadline := time.Now().Add(3 * time.Second)
		for {
			conn, err := net.DialTimeout("tcp", net.JoinHostPort(c.LAN.Address, strconv.Itoa(p.DNSPort())), 200*time.Millisecond)
			if err == nil {
				conn.Close()
				break
			}
			if time.Now().After(deadline) {
				return DevicesView{}, errors.New("Device remains blocked: protected DNS relay is unavailable")
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	for i := range c.Policies {
		if c.Policies[i].ID == id {
			c.Policies[i].Pending = false
		}
	}
	c.Revision = devicepolicy.NewID()
	if err := devicepolicy.PersistIntent(c); err != nil {
		return DevicesView{}, err
	}
	if err := devicepolicy.Save(home, c); err != nil {
		return DevicesView{}, err
	}
	if err := devicepolicy.Apply(home, c); err != nil {
		return DevicesView{}, err
	}
	if err := devicepolicy.Verified(c); err != nil {
		return DevicesView{}, err
	}
	return m.Devices()
}

func (m *Manager) ApplyDevice(req DeviceRequest) (DevicesView, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	unlock, err := devicepolicy.Lock(filepath.Dir(m.path))
	if err != nil {
		return DevicesView{}, err
	}
	defer unlock()
	if err = devicepolicy.CheckCapabilities(); err != nil {
		return DevicesView{}, err
	}
	if m.b.State().Core != "running" {
		return DevicesView{}, errors.New("Start the core before choosing a fixed device egress")
	}
	c, err := m.deviceConfig(req.Revision)
	if err != nil {
		return DevicesView{}, err
	}
	l, err := m.discover()
	if err != nil {
		return DevicesView{}, err
	}
	if len(c.Policies) > 0 && (c.LAN.Device != l.Device || c.LAN.Address != l.Address || c.LAN.Prefix != l.Prefix) {
		return DevicesView{}, errors.New("LAN changed; existing protected devices must be reviewed before adding policies")
	}
	c.LAN = devicepolicy.LAN{Device: l.Device, Address: l.Address, Prefix: l.Prefix}
	nodes, runtime, err := m.deviceCatalog()
	if err != nil {
		return DevicesView{}, err
	}
	if err = devicepolicy.ManagementPortsAvailable(runtime); err != nil {
		return DevicesView{}, err
	}
	if _, ok := devicepolicy.Match(nodes, req.Target); !ok {
		return DevicesView{}, errors.New("Selected node is no longer available; choose a fixed remote node again")
	}
	mac, err := net.ParseMAC(req.MAC)
	if err != nil {
		return DevicesView{}, errors.New("Invalid device MAC address")
	}
	p := devicepolicy.Policy{ID: req.ID, Name: req.Name, MAC: mac.String(), IP: req.IP, Target: req.Target, IPv6: devicepolicy.IPv6Addresses(mac.String())}
	if req.ID != "" {
		old, e := c.Find(req.ID)
		if e != nil {
			return DevicesView{}, e
		}
		// Identity changes require an explicit release, so a failed edit cannot
		// silently remove protection from the old MAC or reserved address.
		if old.MAC != p.MAC || old.IP != p.IP {
			return DevicesView{}, errors.New("Release the existing policy before changing its device identity")
		}
		p.Slot = old.Slot
		p.Paused = old.Paused
		for _, ip := range old.IPv6 {
			found := false
			for _, current := range p.IPv6 {
				if current == ip {
					found = true
				}
			}
			if !found {
				p.IPv6 = append(p.IPv6, ip)
			}
		}
		for i := range c.Policies {
			if c.Policies[i].ID == p.ID {
				c.Policies[i] = p
			}
		}
	} else {
		p.ID = devicepolicy.NewID()
		used := map[int]bool{}
		for _, v := range c.Policies {
			used[v.Slot] = true
		}
		for slot := 1; slot <= devicepolicy.MaxDevices; slot++ {
			if !used[slot] {
				p.Slot = slot
				break
			}
		}
		c.Policies = append(c.Policies, p)
	}
	// A fresh installation has no policy document or revision yet. Assign one
	// before validating the first nonempty policy set, just as commitDevice does
	// for subsequent persisted transaction phases.
	if c.Revision == "" {
		c.Revision = devicepolicy.NewID()
	}
	if err := c.Validate(); err != nil {
		return DevicesView{}, err
	}
	// A stale/static lease belonging to someone else is never overwritten.
	for _, d := range devicepolicy.Discover() {
		if d.IP == p.IP && d.MAC != p.MAC {
			return DevicesView{}, errors.New("This IP address is already assigned to another device")
		}
	}
	return m.commitDevice(c, p.ID, req.Reserve)
}

func (m *Manager) PauseDevice(id, revision string, paused bool) (DevicesView, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	unlock, err := devicepolicy.Lock(filepath.Dir(m.path))
	if err != nil {
		return DevicesView{}, err
	}
	defer unlock()
	c, err := m.deviceConfig(revision)
	if err != nil {
		return DevicesView{}, err
	}
	if _, err = c.Find(id); err != nil {
		return DevicesView{}, err
	}
	for i := range c.Policies {
		if c.Policies[i].ID == id {
			c.Policies[i].Paused = paused
		}
	}
	return m.commitDevice(c, id, false)
}

func (m *Manager) ReleaseDevice(id, revision string) (DevicesView, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	unlock, err := devicepolicy.Lock(filepath.Dir(m.path))
	if err != nil {
		return DevicesView{}, err
	}
	defer unlock()
	c, err := m.deviceConfig(revision)
	if err != nil {
		return DevicesView{}, err
	}
	p, err := c.Find(id)
	if err != nil {
		return DevicesView{}, err
	}
	// First close active proxy sessions under the still-installed restriction.
	if err = m.closeDeviceFlows(p.IP, p.MAC); err != nil {
		return DevicesView{}, err
	}
	policies := []devicepolicy.Policy{}
	for _, v := range c.Policies {
		if v.ID != id {
			policies = append(policies, v)
		}
	}
	c.Policies = policies
	c.Revision = devicepolicy.NewID()
	if err = devicepolicy.PersistIntent(c); err != nil {
		return DevicesView{}, err
	}
	if err = devicepolicy.Save(filepath.Dir(m.path), c); err != nil {
		return DevicesView{}, err
	}
	if err = devicepolicy.Apply(filepath.Dir(m.path), c); err != nil {
		return DevicesView{}, err
	}
	// Release is the sole operation that explicitly permits ordinary routing.
	if err = m.b.Reload(); err != nil {
		return DevicesView{}, errors.New("Protection was released; reload the core to remove the unused listener")
	}
	return m.Devices()
}
