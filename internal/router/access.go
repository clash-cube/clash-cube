package router

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"slices"
	"strings"
)

// AccessPolicy selects devices for transparent interception. It does not
// restrict explicit proxy listeners or change dnsmasq's shared upstream.
type AccessPolicy struct {
	Mode      string   `json:"mode"`
	Addresses []string `json:"addresses"`
	MACs      []string `json:"macs"`
}

func (p AccessPolicy) normalized() (AccessPolicy, error) {
	if p.Mode == "" {
		p.Mode = "all"
	}
	if p.Mode != "all" && p.Mode != "include" && p.Mode != "exclude" {
		return p, errors.New("invalid LAN access mode")
	}
	if len(p.Addresses)+len(p.MACs) > 128 {
		return p, errors.New("LAN access list is limited to 128 entries")
	}
	out := AccessPolicy{Mode: p.Mode, Addresses: []string{}, MACs: []string{}}
	for _, value := range p.Addresses {
		value = strings.TrimSpace(value)
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			address, e := netip.ParseAddr(value)
			if e != nil || !address.Is4() {
				return out, fmt.Errorf("invalid LAN IPv4 address or subnet: %s", value)
			}
			prefix = netip.PrefixFrom(address, 32)
		}
		if !prefix.Addr().Is4() {
			return out, errors.New("LAN access lists currently require IPv4")
		}
		canonical := prefix.Masked().String()
		if !slices.Contains(out.Addresses, canonical) {
			out.Addresses = append(out.Addresses, canonical)
		}
	}
	for _, value := range p.MACs {
		mac, err := net.ParseMAC(strings.TrimSpace(value))
		if err != nil || len(mac) != 6 || mac[0]&1 != 0 {
			return out, errors.New("invalid LAN device MAC address")
		}
		if canonical := mac.String(); !slices.Contains(out.MACs, canonical) {
			out.MACs = append(out.MACs, canonical)
		}
	}
	return out, nil
}

func savePreference(path string, p preference) error {
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path+".tmp", body, 0600); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

// SetAccess changes only firewall rules. Existing core connections, runtime
// configuration and DNS upstreams remain in place during device-list edits.
func (m *Manager) SetAccess(policy AccessPolicy) (Status, error) {
	policy, err := policy.normalized()
	if err != nil {
		return Status{}, err
	}
	m.opMu.Lock()
	defer m.opMu.Unlock()
	capability, err := m.guard.Get()
	if err != nil {
		return capability, err
	}
	if capability.Access.Mode == "" {
		return capability, errors.New("Update the core package before configuring device access")
	}
	before, err := readPreference(m.path)
	if err != nil {
		return Status{}, err
	}
	after := before
	after.Access = policy
	if err := savePreference(m.path, after); err != nil {
		return Status{}, err
	}
	status, err := m.guard.Reconcile()
	if err != nil {
		rollback := savePreference(m.path, before)
		_, reconcile := m.guard.Reconcile()
		return status, errors.Join(err, rollback, reconcile)
	}
	status.Access = policy
	return status, nil
}
