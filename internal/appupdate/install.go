//go:build darwin

package appupdate

import (
	"context"

	"errors"
	"fmt"

	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/localhost-copilot/clashcube/internal/updatesig"
)

// AppName is the bundle's name, in the zip and where it is installed.
const AppName = "ClashCube.app"

// ErrCanceled is the administrator's password prompt dismissed.
var ErrCanceled = errors.New("cancelled")

// Bundle is the .app the running executable lives in, or "" when it is not
// in one (a development build in bin/).
func Bundle() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return ""
	}
	// …/ClashCube.app/Contents/MacOS/clashcube
	app := filepath.Dir(filepath.Dir(filepath.Dir(exe)))
	if filepath.Ext(app) != ".app" || filepath.Base(filepath.Dir(exe)) != "MacOS" {
		return ""
	}
	return app
}

// Stuck says why bundle can't be replaced where it is, or "" when it can:
// opened from its disk image, or moved aside by macOS (App Translocation)
// because it was never moved out of Downloads.
func Stuck(bundle string) string {
	var st unix.Statfs_t
	switch {
	case strings.Contains(bundle, "/AppTranslocation/"):
		return "translocated"
	case unix.Statfs(bundle, &st) == nil && st.Flags&unix.MNT_RDONLY != 0:
		return "read-only"
	}
	return ""
}

// Writable reports whether the app may create files in dir.
func Writable(dir string) bool {
	f, err := os.CreateTemp(dir, ".clashcube-update-*")
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(f.Name())
	return true
}

// StageDir is where an update is unpacked: beside the bundle, so installing
// it is a rename on one volume, or, where the app may not write, in cache,
// to be moved in with the administrator's password.
func StageDir(bundle, cache string) string {
	if dir := filepath.Dir(bundle); Writable(dir) {
		return filepath.Join(dir, ".clashcube-update")
	}
	return filepath.Join(cache, "update")
}

// Stage downloads m's app for arch into dir and unpacks it there. The zip
// must match the hash the signed manifest gives, and its executable must
// carry a signature of the update key from m's build, newer than this one.
// It returns the unpacked app, ready for Install.
func Stage(ctx context.Context, c *http.Client, m *Manifest, arch, dir string, progress func(done, total int64)) (string, error) {
	a, ok := m.Assets[arch]
	if !ok {
		return "", fmt.Errorf("release %s has no build for %s", m.Version, arch)
	}
	if len(a.SHA256) != 64 {
		return "", errors.New("the release lists no checksum for " + arch)
	}
	os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	zip := filepath.Join(dir, "ClashCube.zip")
	if err := download(ctx, c, a, zip, progress); err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	out := filepath.Join(dir, "app")
	if b, err := exec.CommandContext(ctx, "/usr/bin/ditto", "-x", "-k", zip, out).CombinedOutput(); err != nil {
		os.RemoveAll(dir)
		return "", fmt.Errorf("unzip: %v: %s", err, b)
	}
	os.Remove(zip)
	app := filepath.Join(out, AppName)
	if err := check(ctx, app, m.Stamp); err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	return app, nil
}

// check accepts app only with its code seal intact and an executable the
// update key signed at stamp, a build after this one.
func check(ctx context.Context, app string, stamp int64) error {
	if b, err := exec.CommandContext(ctx, "/usr/bin/codesign", "--verify", "--deep", "--strict", app).CombinedOutput(); err != nil {
		return fmt.Errorf("the downloaded app's code seal is broken: %s", strings.TrimSpace(string(b)))
	}
	exe := filepath.Join(app, "Contents", "MacOS", "clashcube")
	if fi, err := os.Lstat(exe); err != nil || !fi.Mode().IsRegular() {
		return errors.New("the downloaded app has no executable")
	}
	bin, err := os.ReadFile(exe)
	if err != nil {
		return err
	}
	sig, err := os.ReadFile(updatesig.SigPath(exe))
	if err != nil {
		return errors.New("the downloaded app is not signed with the update key")
	}
	got, err := updatesig.Verify(pubKey(), bin, sig)
	if err != nil {
		return fmt.Errorf("the downloaded app is not signed with the update key: %w", err)
	}
	if got != stamp {
		return errors.New("the downloaded app is not the build the release names")
	}
	if got <= updatesig.OwnStamp() {
		return errors.New("the downloaded app is not newer than this one")
	}
	return nil
}

// Install swaps the staged app in for bundle. The running copy goes on
// until it quits. Where the folder isn't the user's to change the error is
// one NeedsAdmin recognises, and InstallAsAdmin can do it instead.
func Install(staged, bundle string) error {
	old := filepath.Join(filepath.Dir(staged), "old.app")
	os.RemoveAll(old)
	if err := os.Rename(bundle, old); err != nil {
		return err
	}
	if err := os.Rename(staged, bundle); err != nil {
		os.Rename(old, bundle) // put it back
		return err
	}
	os.RemoveAll(filepath.Dir(filepath.Dir(staged)))
	return nil
}

// NeedsAdmin reports whether err is the app not being allowed to change
// the folder it lives in, or a stage on another volume, which the
// administrator's password gets past.
func NeedsAdmin(err error) bool {
	return errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.EXDEV)
}

// InstallAsAdmin is Install with the administrator's password, asked for
// by the system with prompt.
func InstallAsAdmin(staged, bundle, prompt string) error {
	s, b, o := shq(staged), shq(bundle), shq(filepath.Join(filepath.Dir(staged), "old.app"))
	script := "rm -rf " + o + " && mv " + b + " " + o + " && if mv " + s + " " + b + "; then rm -rf " + o + "; else mv " + o + " " + b + "; exit 1; fi"
	as := "do shell script " + osaq(script) + " with prompt " + osaq(prompt) + " with administrator privileges"
	out, err := exec.Command("/usr/bin/osascript", "-e", as).CombinedOutput()
	if err != nil {
		if msg := strings.TrimSpace(string(out)); strings.Contains(msg, "-128") {
			return ErrCanceled
		} else if msg != "" {
			return errors.New(msg)
		}
		return err
	}
	os.RemoveAll(filepath.Dir(filepath.Dir(staged)))
	return nil
}

// Relaunch opens bundle again once this process has exited.
func Relaunch(bundle string) error {
	script := fmt.Sprintf(`while kill -0 %d 2>/dev/null; do sleep 0.2; done; /usr/bin/open %s`, os.Getpid(), shq(bundle))
	cmd := exec.Command("/bin/sh", "-c", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// shq quotes s for /bin/sh.
func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// osaq quotes s as an AppleScript string.
func osaq(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
