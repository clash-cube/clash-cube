package helper

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

// Install grants this GUI session a helper through UAC. No persistent service
// or machine-wide installation is created; each new session asks again.
func Install(data, prompt string) error {
	if running, current := Installed(); running && current {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	data, err = filepath.Abs(data)
	if err != nil {
		return err
	}
	args := []string{"helper", "serve", "--data", data, "--pipe", SocketPath, "--parent", strconv.Itoa(os.Getpid())}
	for i := range args {
		args[i] = windows.EscapeArg(args[i])
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return err
	}
	params, err := windows.UTF16PtrFromString(strings.Join(args, " "))
	if err != nil {
		return err
	}
	if err := windows.ShellExecute(0, verb, file, params, nil, windows.SW_HIDE); err != nil {
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return errors.New("cancelled")
		}
		return err
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if running, current := Installed(); running && current {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("the elevated helper did not start")
}

// The helper executable is the current app; there is no separate binary to update.
func Uninstall(prompt string) error {
	_, err := call(SocketPath, Request{Op: "stop"})
	return err
}
