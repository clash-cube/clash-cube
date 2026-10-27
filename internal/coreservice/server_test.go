package coreservice

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/localhost-copilot/clashcube/internal/coremgr"
	"github.com/localhost-copilot/clashcube/internal/runtimecfg"
	"github.com/localhost-copilot/clashcube/internal/webui"
)

type unavailableRunner struct{}

func (unavailableRunner) Test(string, string) error { return nil }
func (unavailableRunner) Stop() error               { return nil }
func (unavailableRunner) Start(string, string, runtimecfg.Controller, func(string)) (<-chan error, error) {
	return nil, errors.New("network unavailable")
}

func TestFailedRestorePreservesIntentAcrossRespawn(t *testing.T) {
	for _, restoring := range []bool{false, true} {
		home := t.TempDir()
		config := filepath.Join(home, "runtime.yaml")
		ctl := runtimecfg.Controller{Addr: "127.0.0.1:12345", Secret: "private"}
		if err := os.WriteFile(config, []byte("external-controller: "+ctl.Addr+"\nsecret: "+ctl.Secret+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		s := &supervisor{path: filepath.Join(home, "intent.json"), config: config, core: coremgr.New(unavailableRunner{}, home, config)}
		if err := s.start(ctl, restoring); err == nil {
			t.Fatal("expected unavailable core")
		}
		body, err := os.ReadFile(s.path)
		if err != nil {
			t.Fatal(err)
		}
		var saved record
		if err := json.Unmarshal(body, &saved); err != nil || saved.Running != restoring || saved.Controller != ctl {
			t.Fatalf("restoring=%v: incorrect restart intent: %v", restoring, err)
		}
	}
}

func TestManagementClientCloseOnlyDisconnectsObservation(t *testing.T) {
	// Darwin's Unix socket path limit is shorter than long test temp paths.
	dir, err := os.MkdirTemp("/tmp", "clashcube-service-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "core.sock")
	l, err := webui.Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	var stops atomic.Int32
	ctl := runtimecfg.Controller{Addr: "127.0.0.1:12345", Secret: "private"}
	server := &http.Server{Handler: webui.NewRPCHandler(map[string]any{
		"Core/Status": func() Snapshot { return Snapshot{Status: coremgr.Running, Controller: ctl} },
		"Core/Stop":   func() { stops.Add(1) },
	})}
	go server.Serve(l)
	defer server.Close()
	client := newClient(path)
	status, err := client.Status()
	if err != nil || status.Status != coremgr.Running {
		t.Fatal("could not connect to running service", err)
	}
	exited := client.Observe(ctl)
	client.Close()
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("observer survived management close")
	}
	if stops.Load() != 0 {
		t.Fatal("management close stopped the core")
	}
	client = newClient(path)
	defer client.Close()
	if err := client.Stop(); err != nil {
		t.Fatal(err)
	}
	if stops.Load() != 1 {
		t.Fatal("explicit Stop failed to control core")
	}
}

func TestCoreServiceRejectsUntrustedPathsAndControllerMismatch(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, "runtime.yaml")
	if err := os.WriteFile(config, []byte("external-controller: 127.0.0.1:12345\nsecret: accepted\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s := &supervisor{config: config, core: coremgr.New(&coremgr.LocalRunner{}, home, config)}
	h := webui.NewRPCHandler(map[string]any{"Core/Start": s.Start, "Core/Test": s.Test})
	for _, tc := range []struct {
		path, body string
		code       int
	}{
		{"Core/Start", `[{"Addr":"0.0.0.0:12345","Secret":"accepted"}]`, 422},
		{"Core/Start", `[{"Addr":"127.0.0.1:12345","Secret":"wrong"}]`, 422},
		{"Core/Start", `[{"Addr":"127.0.0.1:12345","Secret":"accepted","path":"/etc/shadow"}]`, 400},
		{"Core/Test", `["/etc/shadow"]`, 400},
		{"Core/Run", `[]`, 404},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/rpc/"+tc.path, strings.NewReader(tc.body)))
		if w.Code != tc.code {
			t.Fatalf("%s: %d, want %d", tc.path, w.Code, tc.code)
		}
	}
}

func TestCoreServiceRecordsIntentPrivately(t *testing.T) {
	s := &supervisor{path: filepath.Join(t.TempDir(), "core-service.json")}
	before := record{Running: true, Controller: runtimecfg.Controller{Addr: "127.0.0.1:12345", Secret: "private"}}
	if err := s.save(before); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(s.path)
	if err != nil || fi.Mode().Perm() != 0600 {
		t.Fatal("controller credentials are not private")
	}
	body, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	var after record
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&after); err != nil || after != before {
		t.Fatal("service restart lost desired core identity")
	}
}

func TestRouterBootHonorsAutoStartAndCurrentRuntime(t *testing.T) {
	home := t.TempDir()
	s := &supervisor{path: filepath.Join(home, "intent.json"), config: filepath.Join(home, "runtime.yaml"), initialized: true}
	old := record{Running: true, Controller: runtimecfg.Controller{Addr: "127.0.0.1:10000", Secret: "old"}}
	if err := s.save(old); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.config, []byte("external-controller: 127.0.0.1:20000\nsecret: current\n"), 0600); err != nil {
		t.Fatal(err)
	}
	stopped, err := s.bootRecord(false, old)
	if err != nil || stopped.Running {
		t.Fatal("AutoStart off restored a running core", err)
	}
	body, _ := os.ReadFile(s.path)
	var persisted record
	if err := json.Unmarshal(body, &persisted); err != nil || persisted.Running {
		t.Fatal("later service respawn would undo AutoStart off")
	}
	running, err := s.bootRecord(true, stopped)
	if err != nil || !running.Running || running.Controller.Secret != "current" {
		t.Fatal("boot did not use the current validated runtime", err)
	}
	body, err = os.ReadFile(s.path)
	if err != nil || json.Unmarshal(body, &persisted) != nil || persisted != running {
		t.Fatal("respawn during boot validation would lose AutoStart intent", err)
	}
	if err := os.Remove(s.config); err != nil {
		t.Fatal(err)
	}
	fresh, err := s.bootRecord(true, running)
	if err != nil || fresh.Running || s.initialized {
		t.Fatal("missing runtime did not defer first setup to management", err)
	}
}
