package backend

import (
	"os/exec"
	"regexp"
	"strings"
	"syscall"
)

func bindInterface(fd uintptr, index int) error {
	return syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, syscall.IP_BOUND_IF, index)
}

var interfaceRe = regexp.MustCompile(`interface:\s*(\S+)`)

// physicalInterface is the interface the router is reached through; under
// TUN the default route is the tunnel, but the gateway stays on the LAN.
func physicalInterface() string {
	gw := gateway()
	if gw == "" {
		return ""
	}
	out, err := exec.Command("/sbin/route", "-n", "get", gw).Output()
	if err != nil {
		return ""
	}
	if m := interfaceRe.FindSubmatch(out); m != nil && !strings.HasPrefix(string(m[1]), "utun") {
		return string(m[1])
	}
	return ""
}

var hardwarePortRe = regexp.MustCompile(`\(Hardware Port: ([^,]+), Device: ([^)]+)\)`)

// serviceOf is the network service on a device, as System Settings names
// it ("Wi-Fi"); "" when none is.
func serviceOf(device string) string {
	out, err := exec.Command("/usr/sbin/networksetup", "-listnetworkserviceorder").Output()
	if err != nil {
		return ""
	}
	for _, m := range hardwarePortRe.FindAllStringSubmatch(string(out), -1) {
		if m[2] == device {
			return m[1]
		}
	}
	return ""
}
func primaryNetwork() string {
	cmd := exec.Command("/usr/sbin/scutil")
	cmd.Stdin = strings.NewReader("show State:/Network/Global/IPv4\n")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return parseNetwork(string(out))
}

func exclusivePort(fd uintptr) error {
	return syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 0)
}
