package helper

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWindowsHelperRefusesPathsAndControllers(t *testing.T) {
	data := t.TempDir()
	home := filepath.Join(data, "core")
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(home, "runtime.yaml")
	if err := os.WriteFile(config, []byte("mixed-port: 0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	valid := Request{Home: home, Config: config, Ctl: "127.0.0.1:12345", Secret: strings.Repeat("s", 32)}
	if err := validateWindowsStart(data, valid); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Request){
		func(r *Request) { r.Home = data },
		func(r *Request) { r.Config = filepath.Join(home, "other.yaml") },
		func(r *Request) { r.Config = filepath.Join(data, "settings.json") },
		func(r *Request) { r.Ctl = "0.0.0.0:12345" },
		func(r *Request) { r.Ctl = "127.0.0.1:0" },
		func(r *Request) { r.Secret = "" },
	} {
		req := valid
		change(&req)
		if validateWindowsStart(data, req) == nil {
			t.Errorf("accepted %+v", req)
		}
	}
	// No elevation is requested for this test. Developer mode may allow links.
	t.Run("reparse-point", func(t *testing.T) {
		target := filepath.Join(data, "other.yaml")
		if err := os.WriteFile(target, []byte("mixed-port: 0\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(config); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, config); err != nil {
			t.Skipf("symlinks unavailable without developer mode: %v", err)
		}
		if validateWindowsStart(data, valid) == nil {
			t.Fatal("accepted runtime.yaml symlink")
		}
	})
}

func TestWindowsHelperProtocolRefusesExecutionAndUpdate(t *testing.T) {
	s := &windowsServer{data: t.TempDir(), version: "test", hash: "test-hash"}
	for _, op := range []string{"exec", "update", "start", "version"} {
		t.Run(op, func(t *testing.T) {
			client, server := net.Pipe()
			defer client.Close()
			client.SetDeadline(time.Now().Add(2 * time.Second))
			go s.handle(server)
			if err := json.NewEncoder(client).Encode(Request{Op: op, Path: `C:\Windows\system32\cmd.exe`}); err != nil {
				t.Fatal(err)
			}
			var response Response
			if err := json.NewDecoder(client).Decode(&response); err != nil {
				t.Fatal(err)
			}
			if response.OK != (op == "version") {
				t.Fatalf("unexpected response: %+v", response)
			}
		})
	}
}
