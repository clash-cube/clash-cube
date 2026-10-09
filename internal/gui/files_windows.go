package gui

import (
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"unsafe"
)

const processEntrySize = unsafe.Sizeof(windows.ProcessEntry32{})

func appsDirectory() string { return os.Getenv("ProgramFiles") }

func openPath(path string, reveal, edit bool) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}
	verb, _ := windows.UTF16PtrFromString("open")
	file := path
	params := ""
	if reveal {
		file = filepath.Join(os.Getenv("SystemRoot"), "explorer.exe")
		params = `/select,"` + path + `"`
	} else if edit {
		file = filepath.Join(os.Getenv("SystemRoot"), "system32", "notepad.exe")
		params = windows.EscapeArg(path)
	}
	f, err := windows.UTF16PtrFromString(file)
	if err != nil {
		return err
	}
	p, err := windows.UTF16PtrFromString(params)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, f, p, nil, windows.SW_SHOWNORMAL)
}
