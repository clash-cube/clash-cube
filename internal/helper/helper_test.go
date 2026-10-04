package helper

import (
	"context"
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
