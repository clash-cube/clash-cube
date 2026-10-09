package backend

import (
	"context"
	"encoding/binary"
	"net"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"github.com/localhost-copilot/clashcube/internal/winutil"
	"golang.org/x/sys/windows"
)

func bindInterface(fd uintptr, index int) error {
	// IP_UNICAST_IF takes the interface index in network byte order.
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], uint32(index))
	return windows.SetsockoptInt(windows.Handle(fd), windows.IPPROTO_IP, 31, int(binary.LittleEndian.Uint32(b[:])))
}

func exclusivePort(fd uintptr) error {
	return windows.SetsockoptInt(windows.Handle(fd), windows.SOL_SOCKET, ^windows.SO_REUSEADDR, 1)
}

func physicalInterface() string      { return winutil.PhysicalAdapter().Name }
func serviceOf(device string) string { return device }
func gateway() string                { return winutil.PhysicalAdapter().Gateway }
func primaryNetwork() string {
	a := winutil.PhysicalAdapter()
	// The watcher uses the first word as an interface name. Windows friendly
	// names contain spaces, so normalize those separators (also used by wifi).
	if a.Name == "" {
		return ""
	}
	return strings.ReplaceAll(a.Name, " ", "_") + " " + a.Gateway
}

func portHolder(port int) string {
	rows, _ := winutil.Listeners()
	for _, row := range rows {
		if row.Port == port {
			if path := winutil.ProcessPath(row.PID); path != "" {
				return filepath.Base(path)
			}
		}
	}
	return ""
}

var icmp = windows.NewLazySystemDLL("iphlpapi.dll")
var icmpCreate = icmp.NewProc("IcmpCreateFile")
var icmpClose = icmp.NewProc("IcmpCloseHandle")
var icmpSend = icmp.NewProc("IcmpSendEcho")

func pingMS(ctx context.Context, host string) float64 {
	ip := net.ParseIP(host).To4()
	if ip == nil || ctx.Err() != nil {
		return -1
	}
	timeout := 2 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		timeout = min(timeout, time.Until(deadline))
	}
	if timeout <= 0 {
		return -1
	}
	h, _, _ := icmpCreate.Call()
	if h == ^uintptr(0) {
		return -1
	}
	defer icmpClose.Call(h)
	reply := make([]byte, 256)
	r, _, _ := icmpSend.Call(h, uintptr(binary.LittleEndian.Uint32(ip)), 0, 0, 0, uintptr(unsafe.Pointer(&reply[0])), uintptr(len(reply)), uintptr(max(1, timeout.Milliseconds())))
	if r == 0 || binary.LittleEndian.Uint32(reply[4:]) != 0 {
		return -1
	}
	return float64(binary.LittleEndian.Uint32(reply[8:]))
}
