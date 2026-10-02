// Package autostart opens MihomoBar at login through a LaunchAgent, which
// (unlike SMAppService) needs no signed bundle.
package autostart

import (
	"bytes"
	"encoding/xml"
	"os"
	"path/filepath"
)

const label = "com.localhost-copilot.mihomobar"

func record() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist")
}

func Enabled() bool {
	_, err := os.Stat(record())
	return err == nil
}

// Set turns open at login on for the running executable, or off.
func Set(on bool) error {
	if !on {
		err := os.Remove(record())
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(record()), 0o755); err != nil {
		return err
	}
	return os.WriteFile(record(), plist(exe), 0o644)
}

func plist(exe string) []byte {
	var p bytes.Buffer
	_ = xml.EscapeText(&p, []byte(exe))
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key><string>` + label + `</string>
	<key>ProgramArguments</key>
	<array><string>` + p.String() + `</string></array>
	<key>RunAtLoad</key><true/>
	<key>LimitLoadToSessionType</key><string>Aqua</string>
	<key>ProcessType</key><string>Interactive</string>
	<key>AbandonProcessGroup</key><true/>
</dict>
</plist>
`)
}
