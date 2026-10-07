//go:build darwin

package appupdate

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/localhost-copilot/clashcube/internal/updatesig"
)

// makeApp bundles a copy of the test binary as ClashCube.app in dir,
// signs its executable with key at stamp, and zips it as a release does.
func makeApp(t *testing.T, dir string, key ed25519.PrivateKey, stamp int64) (zip string) {
	t.Helper()
	app := filepath.Join(dir, AppName)
	exe := filepath.Join(app, "Contents", "MacOS", "clashcube")
	os.MkdirAll(filepath.Dir(exe), 0o755)
	os.MkdirAll(filepath.Join(app, "Contents", "Resources"), 0o755)
	self, _ := os.Executable()
	b, _ := os.ReadFile(self)
	os.WriteFile(exe, b, 0o755)
	os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict><key>CFBundleIdentifier</key><string>test.clashcube</string><key>CFBundleExecutable</key><string>clashcube</string></dict></plist>`), 0o644)
	if out, err := exec.Command("codesign", "--force", "--sign", "-", exe).CombinedOutput(); err != nil {
		t.Fatalf("codesign: %s", out)
	}
	b, _ = os.ReadFile(exe)
	sig, err := updatesig.Sign(key, b, stamp)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(updatesig.SigPath(exe), sig, 0o644)
	if out, err := exec.Command("codesign", "--force", "--deep", "--sign", "-", app).CombinedOutput(); err != nil {
		t.Fatalf("codesign: %s", out)
	}
	zip = filepath.Join(dir, "ClashCube.zip")
	if out, err := exec.Command("ditto", "-c", "-k", "--keepParent", app, zip).CombinedOutput(); err != nil {
		t.Fatalf("ditto: %s", out)
	}
	return zip
}

func serve(t *testing.T, zip string) (*httptest.Server, Asset) {
	b, _ := os.ReadFile(zip)
	sum := sha256.Sum256(b)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(b) }))
	t.Cleanup(srv.Close)
	return srv, Asset{URL: srv.URL, Size: int64(len(b)), SHA256: hex.EncodeToString(sum[:])}
}

func TestStageAndInstall(t *testing.T) {
	key := testKey(t)
	zip := makeApp(t, t.TempDir(), key, 100)
	srv, asset := serve(t, zip)

	installed := filepath.Join(t.TempDir(), AppName)
	os.MkdirAll(filepath.Join(installed, "Contents"), 0o755)
	os.WriteFile(filepath.Join(installed, "Contents", "old"), nil, 0o644)

	m := &Manifest{Version: "9.0.0", Stamp: 100, Assets: map[string]Asset{"arm64": asset}}
	staged, err := Stage(context.Background(), srv.Client(), m, "arm64", StageDir(installed, t.TempDir()), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(staged, installed); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(installed, "Contents", "MacOS", "clashcube")); err != nil {
		t.Error("the new app is not in place")
	}
	if _, err := os.Stat(filepath.Join(installed, "Contents", "old")); err == nil {
		t.Error("the old app is still in place")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(installed), ".clashcube-update")); !os.IsNotExist(err) {
		t.Error("the stage was left behind")
	}
}

func TestStageRefuses(t *testing.T) {
	key := testKey(t)
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	for name, c := range map[string]struct {
		key           ed25519.PrivateKey
		signed, named int64
	}{
		"another key's build":       {other, 100, 100},
		"another build than listed": {key, 100, 101},
	} {
		t.Run(name, func(t *testing.T) {
			zip := makeApp(t, t.TempDir(), c.key, c.signed)
			srv, asset := serve(t, zip)
			m := &Manifest{Version: "9.0.0", Stamp: c.named, Assets: map[string]Asset{"arm64": asset}}
			dir := filepath.Join(t.TempDir(), "stage")
			if _, err := Stage(context.Background(), srv.Client(), m, "arm64", dir, nil); err == nil {
				t.Fatal("staged it")
			}
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Error("left the stage behind")
			}
		})
	}
}
