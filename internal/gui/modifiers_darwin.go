package gui

/*
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

static int optionHeld(void) {
	return ([NSEvent modifierFlags] & NSEventModifierFlagOption) != 0;
}
*/
import "C"

// optionHeld says whether ⌥ is down now, for menu items that do something
// else when option-clicked (as Surge's do).
func optionHeld() bool { return C.optionHeld() != 0 }
