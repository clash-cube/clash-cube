package sysproxy

import (
	"fmt"
	"net"
	"net/netip"
	"runtime"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var wininet = windows.NewLazySystemDLL("wininet.dll")
var setOption = wininet.NewProc("InternetSetOptionW")
var queryOption = wininet.NewProc("InternetQueryOptionW")

// WinINet's per-connection API updates both Internet Settings and the
// connection blob. Editing ProxyEnable alone leaves some clients on stale PAC.
type connectionOption struct {
	Option uint32
	Value  uint64 // union of DWORD, LPWSTR and FILETIME
}
type connectionOptions struct {
	Size       uint32
	Connection *uint16 // nil selects the LAN settings
	Count      uint32
	Error      uint32
	Options    *connectionOption
}

func Set(host string, port int, bypass []string) error {
	if net.ParseIP(host) == nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid proxy endpoint")
	}
	endpoint := net.JoinHostPort(host, strconv.Itoa(port))
	server, err := windows.UTF16PtrFromString("http=" + endpoint + ";https=" + endpoint + ";socks=" + endpoint)
	if err != nil {
		return err
	}
	patterns, err := bypassPatterns(bypass)
	if err != nil {
		return err
	}
	exceptions, err := windows.UTF16PtrFromString(strings.Join(patterns, ";"))
	if err != nil {
		return err
	}
	opts := []connectionOption{{1, 3}, {2, uint64(uintptr(unsafe.Pointer(server)))}, {3, uint64(uintptr(unsafe.Pointer(exceptions)))}}
	err = apply(opts)
	runtime.KeepAlive(server)
	runtime.KeepAlive(exceptions)
	return err
}

// WinINet accepts wildcard IPs, not the CIDRs used in portable settings.
// Expand the partial octet only (at most 128 entries); never broaden a subnet.
func bypassPatterns(entries []string) ([]string, error) {
	var out []string
	for _, entry := range entries {
		if !strings.Contains(entry, "/") {
			out = append(out, entry)
			continue
		}
		prefix, err := netip.ParsePrefix(entry)
		if err != nil || !prefix.Addr().Is4() {
			return nil, fmt.Errorf("Windows proxy bypass requires an IPv4 subnet or a hostname: %s", entry)
		}
		a := prefix.Masked().Addr().As4()
		whole, remainder := prefix.Bits()/8, prefix.Bits()%8
		parts := []string{}
		for i := 0; i < whole; i++ {
			parts = append(parts, strconv.Itoa(int(a[i])))
		}
		if remainder == 0 {
			if whole < 4 {
				parts = append(parts, "*")
			}
			out = append(out, strings.Join(parts, "."))
			continue
		}
		for i := 0; i < 1<<(8-remainder); i++ {
			pattern := append(append([]string{}, parts...), strconv.Itoa(int(a[whole])+i))
			if whole < 3 {
				pattern = append(pattern, "*")
			}
			out = append(out, strings.Join(pattern, "."))
		}
	}
	return out, nil
}

func apply(opts []connectionOption) error {
	list := connectionOptions{Count: uint32(len(opts)), Options: &opts[0]}
	list.Size = uint32(unsafe.Sizeof(list))
	r, _, err := setOption.Call(0, 75, uintptr(unsafe.Pointer(&list)), uintptr(list.Size))
	if r == 0 {
		return fmt.Errorf("set system proxy: %w", err)
	}
	for _, option := range []uintptr{39, 37} {
		if r, _, err := setOption.Call(0, option, 0, 0); r == 0 {
			return fmt.Errorf("refresh system proxy: %w", err)
		}
	}
	return nil
}

func Clear() error                        { return apply([]connectionOption{{1, 1}}) }
func PointsAt(host string, port int) bool { return Effective(host, port) }

func Effective(host string, port int) bool {
	opts := []connectionOption{{Option: 1}, {Option: 2}}
	list := connectionOptions{Count: uint32(len(opts)), Options: &opts[0]}
	list.Size = uint32(unsafe.Sizeof(list))
	size := list.Size
	r, _, _ := queryOption.Call(0, 75, uintptr(unsafe.Pointer(&list)), uintptr(unsafe.Pointer(&size)))
	if r == 0 {
		return false
	}
	// Interpret the native union as a pointer without a uintptr round trip.
	p := *(*unsafe.Pointer)(unsafe.Pointer(&opts[1].Value))
	if p == nil {
		return false
	}
	defer windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalFree").Call(uintptr(p))
	server := windows.UTF16PtrToString((*uint16)(p))
	// PAC or autodetection can override a manual proxy.
	return opts[0].Value&14 == 2 && proxyMatches(server, host, port)
}

func proxyMatches(server, host string, port int) bool {
	want := net.JoinHostPort(host, strconv.Itoa(port))
	if !strings.Contains(server, "=") {
		return strings.TrimSpace(server) == want
	}
	values := map[string]string{}
	for _, part := range strings.Split(server, ";") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok {
			values[strings.ToLower(key)] = strings.TrimSpace(value)
		}
	}
	return values["http"] == want && values["https"] == want
}
