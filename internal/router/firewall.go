// Package router owns the OpenWrt LAN interception lifecycle. It never hooks
// router-originated traffic and never changes the user's ordinary forwarding.
package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"os/exec"
	"strings"
	"time"
)

const chain = "CLASHCUBE_LAN"
const redirectPort = 7898
const dnsPort = 1053

type lan struct {
	Device  string
	Address string
	Prefix  string
}

type command func(string, ...string) ([]byte, error)

func run(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func discoverLAN() (lan, error) {
	body, err := run("ubus", "call", "network.interface.lan", "status")
	if err != nil {
		return lan{}, err
	}
	var status struct {
		Device string `json:"l3_device"`
	}
	if err := json.Unmarshal(body, &status); err != nil {
		return lan{}, err
	}
	iface, err := net.InterfaceByName(status.Device)
	if err != nil {
		return lan{}, err
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return lan{}, err
	}
	for _, addr := range addrs {
		prefix, err := netip.ParsePrefix(addr.String())
		if err == nil && prefix.Addr().Is4() && !prefix.Addr().IsLoopback() {
			return lan{Device: iface.Name, Address: prefix.Addr().String(), Prefix: prefix.Masked().String()}, nil
		}
	}
	return lan{}, fmt.Errorf("LAN has no IPv4 address")
}

// Build a detached chain before installing its hook. Other services' chains
// are never flushed. Cache only our saved rules to detect drift cheaply.
type firewall struct {
	exec       command
	signature  string
	currentLAN lan
	rulesKey   string
}

func ownSignature(body []byte) string {
	var lines []string
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "-A "+chain+" ") || (strings.HasPrefix(line, "-A PREROUTING ") && strings.HasSuffix(line, "-j "+chain)) {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func (f firewall) rules(l lan, policy AccessPolicy) []string {
	var filters []string
	for _, address := range policy.Addresses {
		filters = append(filters, "-s "+address+" ")
	}
	for _, mac := range policy.MACs {
		filters = append(filters, "-m mac --mac-source "+mac+" ")
	}
	var rules []string
	if policy.Mode == "exclude" {
		for _, filter := range filters {
			rules = append(rules, filter+"-j RETURN")
		}
	}
	if policy.Mode != "include" {
		filters = []string{""}
	}
	for _, filter := range filters {
		rules = append(rules, filter+"-p udp --dport 53 -j REDIRECT --to-ports 53", filter+"-p tcp --dport 53 -j REDIRECT --to-ports 53")
	}
	for _, cidr := range []string{l.Address + "/32", "0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24", "192.168.0.0/16", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4"} {
		rules = append(rules, "-d "+cidr+" -j RETURN")
	}
	for _, filter := range filters {
		rules = append(rules, filter+"-p tcp -j REDIRECT --to-ports 7898")
	}
	return rules
}

func (f *firewall) apply(l lan, policy AccessPolicy) error {
	if strings.ContainsAny(l.Device, " \t\r\n") {
		return fmt.Errorf("invalid LAN device")
	}
	if _, err := netip.ParsePrefix(l.Prefix); err != nil {
		return err
	}
	version, err := f.exec("iptables", "--version")
	if err != nil {
		return err
	}
	if !strings.Contains(string(version), "legacy") {
		return fmt.Errorf("LAN proxy currently requires fw3/iptables-legacy")
	}
	body, err := f.exec("iptables-save", "-t", "nat")
	if err != nil {
		return err
	}
	wanted := f.rules(l, policy)
	rulesKey := strings.Join(wanted, "\n")
	jump := "-A PREROUTING -i " + l.Device + " -s " + l.Prefix + " -j " + chain
	if signature := ownSignature(body); signature != "" && signature == f.signature && l == f.currentLAN && rulesKey == f.rulesKey {
		return nil
	}
	f.signature = ""
	// Build/update the detached chain before installing the LAN hook. Holding the
	// manager lock serializes our operations; fw3 reload invokes reconciliation.
	if _, err := f.exec("iptables", "-w", "5", "-t", "nat", "-N", chain); err != nil {
		if _, check := f.exec("iptables", "-w", "5", "-t", "nat", "-S", chain); check != nil {
			return err
		}
	}
	if err := f.removeLinks(); err != nil {
		return err
	}
	if _, err := f.exec("iptables", "-w", "5", "-t", "nat", "-F", chain); err != nil {
		return err
	}
	for _, rule := range wanted {
		if _, err := f.exec("iptables", append([]string{"-w", "5", "-t", "nat", "-A", chain}, strings.Fields(rule)...)...); err != nil {
			return err
		}
	}
	if _, err = f.exec("iptables", append([]string{"-w", "5", "-t", "nat", "-I", "PREROUTING", "1"}, strings.Fields(strings.TrimPrefix(jump, "-A PREROUTING "))...)...); err != nil {
		return err
	}
	body, err = f.exec("iptables-save", "-t", "nat")
	if err == nil {
		f.signature = ownSignature(body)
		f.currentLAN = l
		f.rulesKey = rulesKey
	}
	return err
}

func (f firewall) removeLinks() error {
	body, err := f.exec("iptables-save", "-t", "nat")
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "-A PREROUTING ") && strings.HasSuffix(line, "-j "+chain) {
			if _, err := f.exec("iptables", append([]string{"-w", "5", "-t", "nat", "-D"}, strings.Fields(strings.TrimPrefix(line, "-A "))...)...); err != nil {
				return err
			}
		}
	}
	return nil
}

func (f *firewall) clear() error {
	f.signature = ""
	if err := f.removeLinks(); err != nil {
		return err
	}
	if _, err := f.exec("iptables", "-w", "5", "-t", "nat", "-S", chain); err != nil {
		return nil
	}
	if _, err := f.exec("iptables", "-w", "5", "-t", "nat", "-F", chain); err != nil {
		return err
	}
	_, err := f.exec("iptables", "-w", "5", "-t", "nat", "-X", chain)
	return err
}
