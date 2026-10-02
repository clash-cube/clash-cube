package helper

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Installed says whether the helper is installed and answering, and runs
// the same executable as this app (so an update reinstalls it).
func Installed() (running, current bool) {
	resp, err := Status()
	if err != nil {
		return false, false
	}
	return true, resp.Hash != "" && resp.Hash == selfHash()
}

func selfHash() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	f, err := os.Open(exe)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	_, _ = io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil))
}

func xmlText(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func plist(uid int, data string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key><string>` + Label + `</string>
	<key>ProgramArguments</key>
	<array>
		<string>` + BinaryPath + `</string>
		<string>helper</string>
		<string>serve</string>
		<string>--uid</string><string>` + strconv.Itoa(uid) + `</string>
		<string>--data</string><string>` + xmlText(data) + `</string>
	</array>
	<key>RunAtLoad</key><true/>
	<key>KeepAlive</key><true/>
	<key>StandardErrorPath</key><string>/var/log/mihomobar-helper.log</string>
	<key>StandardOutPath</key><string>/var/log/mihomobar-helper.log</string>
</dict>
</plist>
`
}

// shq quotes s for /bin/sh.
func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// osaq quotes s as an AppleScript string.
func osaq(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// adminShell runs script as root, after macOS asks for an administrator's
// password (osascript "with administrator privileges").
func adminShell(script, prompt string) error {
	as := "do shell script " + osaq(script) + " with prompt " + osaq(prompt) + " with administrator privileges"
	out, err := exec.Command("/usr/bin/osascript", "-e", as).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if strings.Contains(msg, "-128") {
			return fmt.Errorf("cancelled")
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

// Install copies this executable to /Library/PrivilegedHelperTools and loads
// it as a LaunchDaemon serving the current user, asking for an
// administrator's password once.
func Install(data, prompt string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "mihomobar-helper-*.plist")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(plist(os.Getuid(), data)); err != nil {
		return err
	}
	tmp.Close()

	script := strings.Join([]string{
		"set -e",
		"launchctl bootout system/" + Label + " 2>/dev/null || true",
		"mkdir -p /Library/PrivilegedHelperTools",
		"cp -f " + shq(exe) + " " + BinaryPath,
		"chown root:wheel " + BinaryPath,
		"chmod 755 " + BinaryPath,
		"xattr -c " + BinaryPath + " 2>/dev/null || true",
		"cp -f " + shq(tmp.Name()) + " " + PlistPath,
		"chown root:wheel " + PlistPath,
		"chmod 644 " + PlistPath,
		"launchctl bootstrap system " + PlistPath,
		"launchctl kickstart -k system/" + Label,
	}, "\n")
	if err := adminShell(script, prompt); err != nil {
		return err
	}
	// wait for it to answer
	for i := 0; i < 30; i++ {
		if _, err := Status(); err == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("the helper was installed but does not answer")
}

// Uninstall unloads and removes the helper.
func Uninstall(prompt string) error {
	script := strings.Join([]string{
		"launchctl bootout system/" + Label + " 2>/dev/null || true",
		"rm -f " + PlistPath + " " + BinaryPath + " " + SocketPath,
	}, "\n")
	return adminShell(script, prompt)
}
