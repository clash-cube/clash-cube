// Package devicepolicy owns persistent per-device egress restrictions. A policy
// remains a firewall restriction when management, the supervisor or mihomo exits.
package devicepolicy

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
)

const MaxDevices = 32

type LAN struct {
	Device  string `json:"device"`
	Address string `json:"address"`
	Prefix  string `json:"prefix"`
}

type Target struct {
	Name        string `json:"name"`
	Profile     string `json:"profile"`
	Provider    string `json:"provider"`
	Fingerprint string `json:"fingerprint"`
	Type        string `json:"type"`
}

type Policy struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	MAC     string   `json:"mac"`
	IP      string   `json:"ip"`
	IPv6    []string `json:"ipv6,omitempty"`
	Slot    int      `json:"slot"`
	Target  Target   `json:"target"`
	Paused  bool     `json:"paused"`
	Pending bool     `json:"pending"`
}

func (p Policy) RedirPort() int  { return 18100 + p.Slot }
func (p Policy) DNSPort() int    { return 18200 + p.Slot }
func (p Policy) TunnelPort() int { return 18300 + p.Slot }
func (p Policy) Inbound() string { return "CLASHCUBE-DEVICE-" + p.ID }

type Config struct {
	Revision string   `json:"revision"`
	LAN      LAN      `json:"lan"`
	Policies []Policy `json:"policies"`
}

func NewID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func Load(home string) (Config, error) {
	c := Config{Policies: []Policy{}}
	body, err := os.ReadFile(filepath.Join(home, "devices.json"))
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err = json.Unmarshal(body, &c); err != nil {
		return c, errors.New("device policy file is invalid; protection must remain closed")
	}
	return c, c.Validate()
}

func (c Config) Validate() error {
	if c.Revision != "" {
		if _, err := hex.DecodeString(c.Revision); err != nil || len(c.Revision) != 24 {
			return errors.New("invalid device policy revision")
		}
	}
	if len(c.Policies) > MaxDevices {
		return errors.New("at most 32 protected devices are supported")
	}
	if len(c.Policies) == 0 {
		return nil
	}
	if c.Revision == "" {
		return errors.New("invalid device policy revision")
	}
	prefix, err := netip.ParsePrefix(c.LAN.Prefix)
	address, e := netip.ParseAddr(c.LAN.Address)
	if err != nil || e != nil || !prefix.Addr().Is4() || !prefix.Contains(address) || !validInterface(c.LAN.Device) {
		return errors.New("invalid protected LAN")
	}
	macs, ips, slots, ids := map[string]bool{}, map[string]bool{}, map[int]bool{}, map[string]bool{}
	for _, p := range c.Policies {
		for _, s := range p.IPv6 {
			a, e := netip.ParseAddr(s)
			if e != nil || !a.Is6() || a.Is4In6() || a.String() != s {
				return errors.New("invalid remembered IPv6 address")
			}
		}
		if _, err := hex.DecodeString(p.ID); err != nil || len(p.ID) != 24 {
			return errors.New("invalid device policy ID")
		}
		mac, err := net.ParseMAC(p.MAC)
		ip, e := netip.ParseAddr(p.IP)
		if err != nil || len(mac) != 6 || mac[0]&1 != 0 || p.MAC == "00:00:00:00:00:00" {
			return errors.New("invalid device MAC address")
		}
		if e != nil || !ip.Is4() || !prefix.Contains(ip) || ip == address || ip == prefix.Masked().Addr() {
			return errors.New("device IP must be a host in the LAN subnet")
		}
		v := ip.As4()
		next := uint32(v[0])<<24 | uint32(v[1])<<16 | uint32(v[2])<<8 | uint32(v[3])
		mask := uint32(0xffffffff) << uint(32-prefix.Bits())
		if next|mask == 0xffffffff {
			return errors.New("device IP must be a host in the LAN subnet")
		}
		if strings.TrimSpace(p.Name) == "" || len(p.Name) > 128 || strings.ContainsAny(p.Name, "\x00\r\n") {
			return errors.New("invalid device name")
		}
		if p.Slot < 1 || p.Slot > MaxDevices || p.Target.Name == "" || p.Target.Profile == "" || len(p.Target.Fingerprint) != 64 {
			return errors.New("invalid fixed device egress")
		}
		if macs[mac.String()] || ips[p.IP] || slots[p.Slot] || ids[p.ID] {
			return errors.New("device MAC, IP and policy slot must be unique")
		}
		macs[mac.String()], ips[p.IP], slots[p.Slot], ids[p.ID] = true, true, true, true
	}
	return nil
}

func validInterface(s string) bool {
	if s == "" || len(s) > 15 {
		return false
	}
	for _, r := range s {
		if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_.:-", r) {
			return false
		}
	}
	return true
}

// AtomicFile fsyncs the new content before rename. A committed policy is also
// the boot-time deny intent, including Pending policies whose reload failed.
func AtomicFile(path string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".device-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(body)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	if dir, e := os.Open(filepath.Dir(path)); e == nil {
		defer dir.Close()
		return dir.Sync()
	}
	return nil
}

func Save(home string, c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	body, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return AtomicFile(filepath.Join(home, "devices.json"), body)
}

func (c Config) Find(id string) (Policy, error) {
	for _, p := range c.Policies {
		if p.ID == id {
			return p, nil
		}
	}
	return Policy{}, fmt.Errorf("device policy no longer exists")
}
