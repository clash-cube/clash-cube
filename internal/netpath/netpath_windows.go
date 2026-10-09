package netpath

import (
	"runtime"
	"syscall"
	"unsafe"

	ole "github.com/go-ole/go-ole"
)

var networkListManager = ole.NewGUID("{DCB00C01-570F-4A9B-8D69-199FDBA5723B}")
var networkCostManager = ole.NewGUID("{DCB00008-570F-4A9B-8D69-199FDBA5723B}")

func Current() Path {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED)
	if err != nil {
		if e, ok := err.(*ole.OleError); !ok || e.Code() != 1 {
			return Path{}
		}
	}
	defer ole.CoUninitialize()
	manager, err := ole.CreateInstance(networkListManager, networkCostManager)
	if err != nil {
		return Path{}
	}
	defer manager.Release()
	// INetworkCostManager inherits IUnknown; GetCost is vtable slot 3.
	vtable := (*[4]uintptr)(unsafe.Pointer(manager.RawVTable))
	var cost uint32
	hr, _, _ := syscall.SyscallN(vtable[3], uintptr(unsafe.Pointer(manager)), uintptr(unsafe.Pointer(&cost)), 0)
	if int32(hr) < 0 {
		return Path{}
	}
	return Path{Expensive: cost&(2|4|0x40000) != 0, Constrained: cost&(0x10000|0x80000) != 0}
}
