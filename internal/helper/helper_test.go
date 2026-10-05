package helper

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/localhost-copilot/clashcube/internal/core"
	"github.com/localhost-copilot/clashcube/internal/mihomoapi"
	"github.com/localhost-copilot/clashcube/internal/runtimecfg"
	"github.com/localhost-copilot/clashcube/internal/updatesig"
)

// The test binary is the core too: the helper starts os.Executable() "core".
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "core" {
		if err := core.Main(os.Args[2:]); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

const cfg = `
mixed-port: 17911
external-controller: 127.0.0.1:17912
secret: s3cret
geodata-mode: false
geo-auto-update: false
proxies: []
rules:
  - MATCH,DIRECT
`

func serveTemp(t *testing.T, uid int) (sock, data string) {
	t.Helper()
	data = t.TempDir()
	os.MkdirAll(filepath.Join(data, "core"), 0o755)
	os.WriteFile(filepath.Join(data, "core", "runtime.yaml"), []byte(cfg), 0o600)
	// short path: unix sockets are limited to ~104 bytes
	dir, _ := os.MkdirTemp("/tmp", "mbh")
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock = filepath.Join(dir, "h.sock")
	s := &server{uid: uid, data: data, version: "test"}
	go s.serve(sock)
	for i := 0; i < 50; i++ {
		if _, err := os.Stat(sock); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Cleanup(s.stop)
	return sock, data
}

func TestHelperRunsCore(t *testing.T) {
	sock, data := serveTemp(t, os.Getuid())
	if resp, err := call(sock, Request{Op: "version"}); err != nil || resp.Version != "test" {
		t.Fatalf("version: %+v %v", resp, err)
	}
	home := filepath.Join(data, "core")
	r := &Runner{Socket: sock}
	var lines []string
	exited, err := r.Start(home, filepath.Join(home, "runtime.yaml"), runtimecfg.Controller{Addr: "127.0.0.1:17912", Secret: "s3cret"}, func(l string) { lines = append(lines, l) })
	if err != nil {
		t.Fatal(err)
	}
	c := mihomoapi.New("127.0.0.1:17912", "s3cret")
	var v string
	for i := 0; i < 50; i++ {
		if v, err = c.Version(context.Background()); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("core did not answer: %v (%s)", err, strings.Join(lines, "\n"))
	}
	t.Log("core version", v)
	if err := r.Stop(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("no exit after stop")
	}
	if _, err := c.Version(context.Background()); err == nil {
		t.Error("core still answers after stop")
	}
}

func TestHelperRefusesOtherPaths(t *testing.T) {
	sock, _ := serveTemp(t, os.Getuid())
	other := t.TempDir()
	r := &Runner{Socket: sock}
	_, err := r.Start(other, filepath.Join(other, "runtime.yaml"), runtimecfg.Controller{Addr: "127.0.0.1:17913"}, func(string) {})
	if err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("started a core outside the data dir: %v", err)
	}
}

func TestHelperRefusesOtherUsers(t *testing.T) {
	sock, _ := serveTemp(t, os.Getuid()+12345)
	c, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	json.NewEncoder(c).Encode(Request{Op: "version"})
	var resp Response
	json.NewDecoder(c).Decode(&resp)
	if resp.OK || resp.Error != "not allowed" {
		t.Fatalf("a stranger was answered: %+v", resp)
	}
}

func TestPlistIsValid(t *testing.T) {
	f := filepath.Join(t.TempDir(), "p.plist")
	os.WriteFile(f, []byte(plist(501, "/tmp/a b/Application Support/ClashCube & co")), 0o644)
	if out, err := execOut("plutil", "-lint", f); err != nil {
		t.Fatalf("%s", out)
	}
}

// updateServer is a helper whose own executable is a temporary file, with
// a test key.
func updateServer(t *testing.T) (s *server, priv ed25519.PrivateKey, app string, exited *bool) {
	t.Helper()
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	dir := t.TempDir()
	self := filepath.Join(dir, "helper")
	os.WriteFile(self, []byte("old"), 0o755)
	exited = new(bool)
	s = &server{uid: os.Getuid(), data: dir, version: "test", self: self, key: pub, stamp: 100, exit: func(int) { *exited = true }}
	app = filepath.Join(dir, "ClashCube.app", "Contents", "MacOS", "clashcube")
	os.MkdirAll(filepath.Dir(app), 0o755)
	os.MkdirAll(filepath.Dir(updatesig.SigPath(app)), 0o755)
	exe, _ := os.Executable()
	b, _ := os.ReadFile(exe)
	os.WriteFile(app, b, 0o755)
	// Match bundle.sh even when the Intel linker leaves this binary unsigned.
	if out, err := execOut("codesign", "--force", "--sign", "-", app); err != nil {
		t.Fatalf("codesign: %v: %s", err, out)
	}
	return s, priv, app, exited
}

func signApp(t *testing.T, priv ed25519.PrivateKey, app string, stamp int64) {
	t.Helper()
	b, _ := os.ReadFile(app)
	sig, err := updatesig.Sign(priv, b, stamp)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(updatesig.SigPath(app), sig, 0o644)
}

func TestHelperUpdatesItself(t *testing.T) {
	s, priv, app, _ := updateServer(t)
	signApp(t, priv, app, 200)
	if err := s.update(app); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(s.self)
	want, _ := os.ReadFile(app)
	if string(got) != string(want) {
		t.Fatal("the helper was not replaced")
	}
	if fi, _ := os.Stat(s.self); fi.Mode().Perm() != 0o755 {
		t.Errorf("mode %v", fi.Mode())
	}
}

func TestHelperRefusesBadUpdates(t *testing.T) {
	cases := map[string]func(t *testing.T, s *server, priv ed25519.PrivateKey, app string) string{
		"unsigned": func(t *testing.T, s *server, priv ed25519.PrivateKey, app string) string {
			return app
		},
		"another key": func(t *testing.T, s *server, _ ed25519.PrivateKey, app string) string {
			_, other, _ := ed25519.GenerateKey(rand.Reader)
			signApp(t, other, app, 200)
			return app
		},
		"older": func(t *testing.T, s *server, priv ed25519.PrivateKey, app string) string {
			signApp(t, priv, app, 50)
			return app
		},
		"same build": func(t *testing.T, s *server, priv ed25519.PrivateKey, app string) string {
			signApp(t, priv, app, 100)
			return app
		},
		"changed after signing": func(t *testing.T, s *server, priv ed25519.PrivateKey, app string) string {
			signApp(t, priv, app, 200)
			f, _ := os.OpenFile(app, os.O_WRONLY, 0)
			f.WriteAt([]byte{0xff, 0xff}, 4096)
			f.Close()
			return app
		},
		"symlink": func(t *testing.T, s *server, priv ed25519.PrivateKey, app string) string {
			signApp(t, priv, app, 200)
			link := filepath.Join(filepath.Dir(app), "link")
			os.Symlink(app, link)
			return link
		},
		"relative path": func(t *testing.T, s *server, priv ed25519.PrivateKey, app string) string {
			signApp(t, priv, app, 200)
			return "clashcube"
		},
		"directory": func(t *testing.T, s *server, priv ed25519.PrivateKey, app string) string {
			return filepath.Dir(app)
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			s, priv, app, _ := updateServer(t)
			path := setup(t, s, priv, app)
			if err := s.update(path); err == nil {
				t.Fatal("accepted")
			}
			if b, _ := os.ReadFile(s.self); string(b) != "old" {
				t.Fatal("the helper was replaced")
			}
		})
	}
}

func TestHelperRefusesOtherUsersFiles(t *testing.T) {
	s, priv, app, _ := updateServer(t)
	signApp(t, priv, app, 200)
	s.uid = os.Getuid() + 12345
	if os.Getuid() == 0 {
		t.Skip("files are root's")
	}
	if err := s.update(app); err == nil || !strings.Contains(err.Error(), "not the user's") {
		t.Fatalf("accepted another user's file: %v", err)
	}
}

func TestHelperUpdateOverSocket(t *testing.T) {
	s, priv, app, exited := updateServer(t)
	signApp(t, priv, app, 200)
	dir, _ := os.MkdirTemp("/tmp", "mbh")
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "h.sock")
	go s.serve(sock)
	for i := 0; i < 50; i++ {
		if _, err := os.Stat(sock); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := call(sock, Request{Op: "update", Path: app + ".missing"}); err == nil {
		t.Fatal("accepted a missing file")
	}
	if *exited {
		t.Fatal("exited after a refusal")
	}
	if _, err := call(sock, Request{Op: "update", Path: app}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if !*exited {
		t.Fatal("did not restart after updating")
	}
}
