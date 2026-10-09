// Package winutil contains native Windows queries shared by the GUI and core.
package winutil

import (
	"encoding/binary"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var iphelper = windows.NewLazySystemDLL("iphlpapi.dll")
var tcpTable = iphelper.NewProc("GetExtendedTcpTable")

type Listener struct {
	Port int
	PID  uint32
}

// Listeners uses the owner-PID table, including IPv6 sockets. Enumerating file
// descriptors is not meaningful for Winsock handles.
func Listeners() ([]Listener, error) {
	var out []Listener
	for _, af := range []uintptr{windows.AF_INET, windows.AF_INET6} {
		var size uint32
		for tries := 0; tries < 5; tries++ {
			buf := make([]byte, max(size, 4))
			r, _, _ := tcpTable.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0, af, 3, 0)
			if r == uintptr(windows.ERROR_INSUFFICIENT_BUFFER) {
				continue
			}
			if r != 0 {
				return nil, windows.Errno(r)
			}
			stride, portOffset, pidOffset := 24, 8, 20
			if af == windows.AF_INET6 {
				stride, portOffset, pidOffset = 56, 20, 52
			}
			n := int(binary.LittleEndian.Uint32(buf))
			if n > (len(buf)-4)/stride {
				return nil, fmt.Errorf("invalid TCP table size")
			}
			for i := 0; i < n; i++ {
				row := buf[4+i*stride:]
				out = append(out, Listener{int(binary.BigEndian.Uint16(row[portOffset:])), binary.LittleEndian.Uint32(row[pidOffset:])})
			}
			break
		}
	}
	return out, nil
}

type Adapter struct {
	Name, Gateway string
	Index         int
	WiFi          bool
	LUID          uint64
}

// PhysicalAdapter selects an up Ethernet/Wi-Fi adapter with a real gateway.
// Tunnel interfaces never participate, so TUN route changes don't look like
// the physical network changed.
func PhysicalAdapter() Adapter {
	size := uint32(15000)
	for tries := 0; tries < 5; tries++ {
		buf := make([]byte, size)
		first := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
		err := windows.GetAdaptersAddresses(windows.AF_INET, windows.GAA_FLAG_INCLUDE_GATEWAYS, 0, first, &size)
		if err == windows.ERROR_BUFFER_OVERFLOW {
			continue
		}
		if err != nil {
			return Adapter{}
		}
		var best Adapter
		metric := ^uint32(0)
		for a := first; a != nil; a = a.Next {
			if a.OperStatus != windows.IfOperStatusUp || (a.IfType != 6 && a.IfType != 71) || a.Ipv4Metric >= metric {
				continue
			}
			for g := a.FirstGatewayAddress; g != nil; g = g.Next {
				ip := g.Address.IP()
				if ip == nil || ip.IsUnspecified() {
					continue
				}
				best = Adapter{windows.UTF16PtrToString(a.FriendlyName), ip.String(), int(a.IfIndex), a.IfType == 71, a.Luid}
				metric = a.Ipv4Metric
				break
			}
		}
		return best
	}
	return Adapter{}
}

func ProcessPath(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, 32768)
	n := uint32(len(buf))
	if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}
