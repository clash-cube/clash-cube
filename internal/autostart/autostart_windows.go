package autostart

import (
	"errors"
	"golang.org/x/sys/windows/registry"
	"os"
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const approvedKey = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`
const name = "ClashCube"

func Enabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	if value, _, err := k.GetStringValue(name); err != nil || value == "" {
		return false
	}
	if a, err := registry.OpenKey(registry.CURRENT_USER, approvedKey, registry.QUERY_VALUE); err == nil {
		defer a.Close()
		if b, _, err := a.GetBinaryValue(name); err == nil && len(b) > 0 && b[0]&1 != 0 {
			return false
		}
	}
	return true
}

func Set(on bool) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !on {
		err := k.DeleteValue(name)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := k.SetStringValue(name, `"`+exe+`"`); err != nil {
		return err
	}
	if a, err := registry.OpenKey(registry.CURRENT_USER, approvedKey, registry.SET_VALUE); err == nil {
		defer a.Close()
		if err := a.DeleteValue(name); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return err
		}
	}
	return nil
}
