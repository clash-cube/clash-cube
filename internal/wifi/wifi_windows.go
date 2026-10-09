package wifi

import (
	"encoding/binary"
	"strings"
	"unsafe"

	"github.com/localhost-copilot/clashcube/internal/winutil"
	"golang.org/x/sys/windows"
)

var wlan = windows.NewLazySystemDLL("wlanapi.dll")
var openHandle = wlan.NewProc("WlanOpenHandle")
var closeHandle = wlan.NewProc("WlanCloseHandle")
var freeMemory = wlan.NewProc("WlanFreeMemory")
var queryInterface = wlan.NewProc("WlanQueryInterface")
var luidToGUID = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("ConvertInterfaceLuidToGuid")

func Current() Status {
	a := winutil.PhysicalAdapter()
	if !a.WiFi {
		return Status{State: "disconnected"}
	}
	st := Status{State: "unavailable", Interface: strings.ReplaceAll(a.Name, " ", "_")}
	var negotiated uint32
	var handle windows.Handle
	r, _, _ := openHandle.Call(2, 0, uintptr(unsafe.Pointer(&negotiated)), uintptr(unsafe.Pointer(&handle)))
	if r != 0 {
		return st
	}
	defer closeHandle.Call(uintptr(handle), 0)
	var guid windows.GUID
	r, _, _ = luidToGUID.Call(uintptr(unsafe.Pointer(&a.LUID)), uintptr(unsafe.Pointer(&guid)))
	if r != 0 {
		return st
	}
	var size, opcodeType uint32
	var data unsafe.Pointer
	r, _, _ = queryInterface.Call(uintptr(handle), uintptr(unsafe.Pointer(&guid)), 7, 0, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&data)), uintptr(unsafe.Pointer(&opcodeType)))
	if r == uintptr(windows.ERROR_ACCESS_DENIED) {
		st.State = "denied"
		return st
	}
	if r != 0 {
		return st
	}
	defer freeMemory.Call(uintptr(data))
	// WLAN_CONNECTION_ATTRIBUTES begins with state, mode and a WCHAR[256]
	// profile name, followed by WLAN_ASSOCIATION_ATTRIBUTES.dot11Ssid.
	if data == nil || size < 556 {
		return st
	}
	buf := unsafe.Slice((*byte)(data), size)
	n := binary.LittleEndian.Uint32(buf[520:])
	if n > 32 {
		return st
	}
	st.State, st.SSID = "connected", string(buf[524:524+n])
	return st
}

func RequestPermission() {
	verb, _ := windows.UTF16PtrFromString("open")
	uri, _ := windows.UTF16PtrFromString("ms-settings:privacy-location")
	_ = windows.ShellExecute(0, verb, uri, nil, nil, windows.SW_SHOWNORMAL)
}

// Saved profiles are not scanned; the current SSID is enough to create a rule.
func SavedNetworks() ([]string, error) {
	if st := Current(); st.SSID != "" {
		return []string{st.SSID}, nil
	}
	return []string{}, nil
}
