package devicepolicy

import (
	"encoding/json"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"sort"
	"strings"
)

type Device struct {
	CurrentIP string `json:"currentIP,omitempty"`
	Name      string `json:"name"`
	MAC       string `json:"mac"`
	IP        string `json:"ip"`
	Reserved  bool   `json:"reserved"`
	Online    bool   `json:"online"`
}

func Discover() []Device {
	devices := map[string]Device{}
	if body, err := os.ReadFile("/tmp/dhcp.leases"); err == nil {
		for _, line := range strings.Split(string(body), "\n") {
			f := strings.Fields(line)
			if len(f) >= 4 {
				devices[strings.ToLower(f[1])] = Device{Name: f[3], MAC: strings.ToLower(f[1]), IP: f[2]}
			}
		}
	}
	if body, err := exec.Command("ubus", "call", "uci", "get", `{"config":"dhcp"}`).Output(); err == nil {
		var doc struct {
			Values map[string]map[string]any `json:"values"`
		}
		_ = json.Unmarshal(body, &doc)
		for _, v := range doc.Values {
			if v[".type"] != "host" {
				continue
			}
			ip, _ := v["ip"].(string)
			name, _ := v["name"].(string)
			macs := []string{}
			switch m := v["mac"].(type) {
			case string:
				macs = strings.Fields(m)
			case []any:
				for _, s := range m {
					if s, ok := s.(string); ok {
						macs = append(macs, s)
					}
				}
			}
			for _, mac := range macs {
				m, err := net.ParseMAC(mac)
				if err == nil {
					devices[m.String()] = Device{Name: name, MAC: m.String(), IP: ip, Reserved: true}
				}
			}
		}
	}
	if body, err := exec.Command("ip", "-j", "-4", "neigh", "show").Output(); err == nil {
		var entries []struct {
			IP    string   `json:"dst"`
			MAC   string   `json:"lladdr"`
			State []string `json:"state"`
		}
		_ = json.Unmarshal(body, &entries)
		for _, v := range entries {
			if v.MAC == "" {
				continue
			}
			mac := strings.ToLower(v.MAC)
			d := devices[mac]
			d.MAC = mac
			if d.IP == "" {
				d.IP = v.IP
			}
			for _, s := range v.State {
				if s != "FAILED" && s != "INCOMPLETE" {
					d.Online = true
				}
				if s == "REACHABLE" {
					d.CurrentIP = v.IP
				}
			}
			devices[mac] = d
		}
	}
	out := []Device{}
	for _, d := range devices {
		if d.Name == "" || d.Name == "*" {
			d.Name = d.IP
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].MAC < out[j].MAC
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func IPv6Addresses(mac string) []string {
	addresses := []string{}
	body, err := exec.Command("ip", "-j", "-6", "neigh", "show").Output()
	if err != nil {
		return addresses
	}
	var entries []struct {
		IP  string `json:"dst"`
		MAC string `json:"lladdr"`
	}
	if json.Unmarshal(body, &entries) != nil {
		return addresses
	}
	for _, v := range entries {
		if strings.EqualFold(v.MAC, mac) {
			if a, err := netip.ParseAddr(v.IP); err == nil && a.Is6() && !a.Is4In6() {
				addresses = append(addresses, a.String())
			}
		}
	}
	return addresses
}

func LocalHosts() map[string]net.IP {
	hosts := map[string]net.IP{"localhost": net.ParseIP("127.0.0.1")}
	for _, d := range Discover() {
		hosts[strings.ToLower(d.Name)] = net.ParseIP(d.IP)
		hosts[strings.ToLower(d.Name)+".lan"] = net.ParseIP(d.IP)
	}
	if body, err := os.ReadFile("/etc/hosts"); err == nil {
		for _, line := range strings.Split(string(body), "\n") {
			f := strings.Fields(strings.SplitN(line, "#", 2)[0])
			if len(f) > 1 {
				for _, host := range f[1:] {
					hosts[strings.ToLower(host)] = net.ParseIP(f[0])
				}
			}
		}
	}
	return hosts
}
