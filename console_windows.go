package main

import (
	"os"
	"syscall"
)

// Desktop builds have no console. Borrow a terminal only for CLI commands,
// preserving redirected streams and the core's lifetime pipe.
func init() {
	if len(os.Args) < 2 || os.Args[1] == "core" || os.Args[1] == "helper" {
		return
	}
	if kind, err := syscall.GetFileType(syscall.Stdout); err == nil && kind != 0 {
		return
	}
	r, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("AttachConsole").Call(^uintptr(0))
	if r == 0 {
		return
	}
	if out, err := os.OpenFile("CONOUT$", os.O_RDWR, 0); err == nil {
		os.Stdout, os.Stderr = out, out
	}
}
