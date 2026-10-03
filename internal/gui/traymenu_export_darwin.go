package gui

// The callback lives apart from traymenu_darwin.go: a file with //export
// may only declare in its preamble, not define.

import "C"

// trayMenuTracking is AppKit telling that the tray menu opened or closed.
//
//export trayMenuTracking
func trayMenuTracking(open C.int) {
	if m := theTrayMenu; m != nil {
		m.tracking(open != 0)
	}
}
