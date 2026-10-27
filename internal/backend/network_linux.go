package backend

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

type linuxRoute struct {
	Gateway string `json:"gateway"`
	Device  string `json:"dev"`
}

func defaultRoute() linuxRoute {
	out, err := exec.Command("ip", "-j", "-4", "route", "show", "default").Output()
	if err != nil {
		return linuxRoute{}
	}
	var routes []linuxRoute
	if json.Unmarshal(out, &routes) != nil || len(routes) == 0 {
		return linuxRoute{}
	}
	return routes[0]
}

func gateway() string                { return defaultRoute().Gateway }
func physicalInterface() string      { return defaultRoute().Device }
func serviceOf(device string) string { return device }
func primaryNetwork() string {
	r := defaultRoute()
	if r.Device == "" {
		return ""
	}
	return r.Device + ":" + r.Gateway
}

func bindInterface(fd uintptr, index int) error {
	i, err := net.InterfaceByIndex(index)
	if err != nil {
		return err
	}
	return syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, i.Name)
}

func exclusivePort(fd uintptr) error {
	return syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 0)
}

// Resolve listener inodes through procfs rather than depending on ss/lsof,
// which small OpenWrt images do not include. Processes may exit during a scan.
func portHolder(port int) string {
	inodes := map[string]bool{}
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		body, _ := os.ReadFile(path)
		for _, line := range strings.Split(string(body), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 10 || fields[3] != "0A" {
				continue
			}
			_, raw, ok := strings.Cut(fields[1], ":")
			p, err := strconv.ParseInt(raw, 16, 32)
			if ok && err == nil && int(p) == port {
				inodes[fields[9]] = true
			}
		}
	}
	if len(inodes) == 0 {
		return ""
	}
	processes, _ := os.ReadDir("/proc")
	for _, process := range processes {
		if _, err := strconv.Atoi(process.Name()); err != nil {
			continue
		}
		base := filepath.Join("/proc", process.Name())
		fds, _ := os.ReadDir(filepath.Join(base, "fd"))
		for _, fd := range fds {
			link, _ := os.Readlink(filepath.Join(base, "fd", fd.Name()))
			inode := strings.TrimSuffix(strings.TrimPrefix(link, "socket:["), "]")
			if !strings.HasPrefix(link, "socket:[") || !inodes[inode] {
				continue
			}
			name, _ := os.ReadFile(filepath.Join(base, "comm"))
			return strings.TrimSpace(string(name)) + " (" + process.Name() + ")"
		}
	}
	return ""
}

var pingRe = regexp.MustCompile(`time[=<]([\d.]+)\s*ms`)

func pingMS(ctx context.Context, host string) float64 {
	out, err := exec.CommandContext(ctx, "ping", "-c", "1", "-W", "2", "-n", host).Output()
	if err != nil {
		return -1
	}
	m := pingRe.FindSubmatch(out)
	if m == nil {
		return -1
	}
	n, err := strconv.ParseFloat(string(m[1]), 64)
	if err != nil {
		return -1
	}
	return roundMS(n)
}
