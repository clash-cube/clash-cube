package devicepolicy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const table = "clashcube_guard"
const expectedPath = "/var/run/clashcube-device-guard.json"
const BootHome = "/etc/clashcube-guard"

// PersistIntent precedes the main settings write. This tiny flash-resident
// marker lets early boot fail closed even when external data storage is absent.
func PersistIntent(c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	body, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return AtomicFile(filepath.Join(BootHome, "intent.json"), body)
}

func nft(input string, args ...string) ([]byte, error) {
	c := exec.Command("nft", args...)
	c.Stdin = strings.NewReader(input)
	out, err := c.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("device firewall: %s", strings.TrimSpace(string(out)))
	}
	return out, nil
}

// Rules is a complete atomic transaction. An independent table deliberately
// outlives both procd services and fw3's own iptables chains.
func Rules(c Config) (string, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# clashcube-device-revision %s\nadd table inet %s\ndelete table inet %s\n", c.Revision, table, table)
	if len(c.Policies) == 0 {
		return b.String(), nil
	}
	fmt.Fprintf(&b, "table inet %s {\n comment \"%s\"\n", table, c.Revision)
	for _, p := range c.Policies {
		fmt.Fprintf(&b, "chain d%d {\n", p.Slot)
		// The client may keep DHCP, neighbour discovery and LuCI/SSH access.
		// All DNS, including requests to the router, must use its private relay.
		fmt.Fprintf(&b, "ip saddr %s ether saddr %s ct status dnat meta l4proto { tcp, udp } th dport %d accept\n", p.IP, p.MAC, p.DNSPort())
		if !p.Paused && !p.Pending {
			fmt.Fprintf(&b, "ip saddr %s ether saddr %s ct status dnat tcp dport %d accept\n", p.IP, p.MAC, p.RedirPort())
		}
		fmt.Fprintf(&b, "udp sport 68 udp dport 67 accept\nmeta l4proto ipv6-icmp icmpv6 type { nd-router-solicit, nd-neighbor-solicit, nd-neighbor-advert } accept\nip daddr %s tcp dport { 22, 80, 443 } accept\nip daddr %s icmp type echo-request accept\ndrop\n}\n", c.LAN.Address, c.LAN.Address)
	}
	fmt.Fprintf(&b, "chain input { type filter hook input priority -10; policy accept;\n")
	for _, p := range c.Policies {
		fmt.Fprintf(&b, "iifname != \"lo\" ether saddr %s jump d%d\niifname != \"lo\" ip saddr %s jump d%d\n", p.MAC, p.Slot, p.IP, p.Slot)
	}
	// Dedicated ports are never a public proxy for other LAN/WAN devices.
	for _, p := range c.Policies {
		fmt.Fprintf(&b, "iifname != \"lo\" meta l4proto { tcp, udp } th dport { %d, %d, %d } drop\n", p.RedirPort(), p.DNSPort(), p.TunnelPort())
	}
	b.WriteString("}\nchain forward { type filter hook forward priority -10; policy accept;\n")
	for _, p := range c.Policies {
		for _, source := range []string{"ether saddr " + p.MAC, "ip saddr " + p.IP} {
			fmt.Fprintf(&b, "iifname \"%s\" %s oifname \"%s\" ip daddr %s return\niifname != \"lo\" %s drop\n", c.LAN.Device, source, c.LAN.Device, c.LAN.Prefix, source)
		}
		fmt.Fprintf(&b, "iifname != \"%s\" oifname \"%s\" ip daddr %s drop\n", c.LAN.Device, c.LAN.Device, p.IP)
		for _, ip := range p.IPv6 {
			fmt.Fprintf(&b, "iifname != \"%s\" ip6 daddr %s drop\n", c.LAN.Device, ip)
		}
	}
	b.WriteString("}\nchain prerouting { type nat hook prerouting priority -110; policy accept;\n")
	for _, p := range c.Policies {
		match := fmt.Sprintf("iifname \"%s\" ether saddr %s ip saddr %s", c.LAN.Device, p.MAC, p.IP)
		fmt.Fprintf(&b, "%s meta l4proto { tcp, udp } th dport 53 redirect to :%d\n", match, p.DNSPort())
		if !p.Paused && !p.Pending {
			fmt.Fprintf(&b, "%s ip daddr != %s meta l4proto tcp redirect to :%d\n", match, c.LAN.Prefix, p.RedirPort())
		}
	}
	b.WriteString("}\n}\n")
	return b.String(), nil
}

// Apply writes boot recovery material before changing kernel state. A failed
// apply never acknowledges a protected device and leaves the deny intent saved.
func Apply(home string, c Config) error {
	unlock, err := Lock(BootHome)
	if err != nil {
		return err
	}
	defer unlock()
	rules, err := Rules(c)
	if err != nil {
		return err
	}
	if _, err = nft(rules, "-c", "-f", "-"); err != nil {
		return err
	}
	if err = AtomicFile(filepath.Join(home, "device-guard.nft"), []byte(rules)); err != nil {
		return err
	}
	if err = AtomicFile(filepath.Join(BootHome, "guard.nft"), []byte(rules)); err != nil {
		return err
	}
	if _, err = nft(rules, "-f", "-"); err != nil {
		return err
	}
	if len(c.Policies) == 0 {
		_ = os.Remove(expectedPath)
		return nil
	}
	shape, err := liveShape()
	if err != nil {
		return err
	}
	return AtomicFile(expectedPath, shape)
}

func liveShape() ([]byte, error) {
	body, err := nft("", "-s", "-j", "list", "table", "inet", table)
	if err != nil {
		return nil, err
	}
	return CanonicalShape(body)
}

// Handles and metadata change on reload. Everything describing the actual
// expressions, chain hooks and verdicts remains part of the verification.
func CanonicalShape(body []byte) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	items, _ := doc["nftables"].([]any)
	out := []any{}
	var clean func(any)
	clean = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			delete(x, "handle")
			for _, v := range x {
				clean(v)
			}
		case []any:
			for _, v := range x {
				clean(v)
			}
		}
	}
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok || m["metainfo"] != nil {
			continue
		}
		clean(m)
		out = append(out, m)
	}
	return json.Marshal(out)
}

func Verified(c Config) error {
	expected, err := os.ReadFile(expectedPath)
	if err != nil {
		return errors.New("device protection has not been verified")
	}
	// Boot scripts store raw nft JSON; management stores canonical JSON.
	if len(expected) > 0 && expected[0] == '{' {
		expected, err = CanonicalShape(expected)
		if err != nil {
			return err
		}
	}
	live, err := liveShape()
	if err != nil {
		return err
	}
	if !bytes.Equal(bytes.TrimSpace(expected), live) {
		return errors.New("device firewall differs from the verified rules")
	}
	var items []map[string]json.RawMessage
	if err = json.Unmarshal(live, &items); err != nil {
		return err
	}
	for _, item := range items {
		if body, ok := item["table"]; ok {
			var t struct {
				Comment string `json:"comment"`
			}
			if json.Unmarshal(body, &t) == nil && t.Comment == c.Revision {
				return nil
			}
		}
	}
	return errors.New("device firewall has not applied the latest policy revision")
}

func CheckRules(c Config) error {
	rules, err := Rules(c)
	if err != nil {
		return err
	}
	_, err = nft(rules, "-c", "-f", "-")
	return err
}

func CheckCapabilities() error {
	for _, key := range []string{"flow_offloading", "flow_offloading_hw"} {
		out, _ := exec.Command("uci", "-q", "get", "firewall.@defaults[0]."+key).Output()
		if strings.TrimSpace(string(out)) == "1" {
			return errors.New("Disable software and hardware flow offloading before enabling device protection")
		}
	}
	if _, err := exec.LookPath("nft"); err != nil {
		return errors.New("Install nftables and kmod-nft-nat before enabling device protection")
	}
	if _, err := os.Stat("/etc/init.d/clashcube-guard"); err != nil {
		return errors.New("Install the clashcube-guard package before enabling device protection")
	}
	if _, err := exec.LookPath("flock"); err != nil {
		return errors.New("Install flock before enabling device protection")
	}
	if err := exec.Command("/etc/init.d/clashcube-guard", "enabled").Run(); err != nil {
		return errors.New("Enable the clashcube-guard boot service before enabling device protection")
	}
	return nil
}

func Restore(home string) error {
	unlock, err := Lock(home)
	if err != nil {
		return err
	}
	defer unlock()
	c, err := Load(home)
	if body, e := os.ReadFile(filepath.Join(BootHome, "intent.json")); e == nil {
		var intent Config
		if json.Unmarshal(body, &intent) != nil || intent.Validate() != nil {
			return errors.New("device boot intent is invalid; emergency protection remains closed")
		}
		if err != nil || intent.Revision != c.Revision {
			// Intent is written first. A power failure before the secondary
			// settings write must recover every intended identity, never the
			// older set of devices. Require review before opening their gates.
			c = intent
			c.Revision = NewID()
			for i := range c.Policies {
				c.Policies[i].Pending = true
			}
			if err = PersistIntent(c); err != nil {
				return err
			}
			if err = Save(home, c); err != nil {
				return err
			}
		}
	}
	if err != nil {
		return err
	}
	if len(c.Policies) == 0 {
		return nil
	}
	return Apply(home, c)
}
