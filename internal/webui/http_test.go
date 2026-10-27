package webui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/localhost-copilot/clashcube/internal/backend"
)

func TestRPCBoundary(t *testing.T) {
	called := 0
	type input struct {
		Port int `json:"port"`
	}
	h := handler{events: NewBroker(), methods: map[string]any{
		"Settings/Patch": func(p input) (int, error) { called++; return p.Port, nil },
	}}
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"POST", "/rpc/Settings/Patch", `[{"port":7897}]`, 200},
		{"POST", "/rpc/Settings/Patch", `[{"port":7897,"command":"sh"}]`, 400},
		{"POST", "/rpc/Settings/Patch", `[{"port":"oops"}]`, 400},
		{"POST", "/rpc/Settings/Patch", `[]`, 400},
		{"POST", "/rpc/Settings/Patch", `[{"port":1}] []`, 400},
		{"GET", "/rpc/Settings/Patch", `[]`, 405},
		{"POST", "/rpc/App/EnableServiceMode", `[]`, 404},
		{"POST", "/rpc/Profiles/ImportFile", `["/etc/shadow"]`, 404},
	} {
		t.Run(tc.method+tc.path+tc.body, func(t *testing.T) {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
			if w.Code != tc.status {
				t.Fatalf("status %d: %s", w.Code, w.Body)
			}
			if !json.Valid(w.Body.Bytes()) {
				t.Fatal("invalid JSON")
			}
			if w.Header().Get("Content-Length") == "" {
				t.Fatal("bridge requires a length")
			}
		})
	}
	if called != 1 {
		t.Fatalf("invalid requests invoked method: %d", called)
	}
}

func TestIndependentEventCursorsAndRecovery(t *testing.T) {
	b := NewBroker()
	b.State(backend.State{Core: "running"})
	first := b.Poll(context.Background(), cursor{})
	if !first.Reset || first.Events[0].Name != "state" {
		t.Fatalf("initial snapshot: %+v", first)
	}
	c := cursor{Stream: first.Stream, After: first.After}
	b.Publish("log", "one")
	for range 2 {
		next := b.Poll(context.Background(), c)
		if next.Reset || len(next.Events) != 1 || next.Events[0].Data != "one" {
			t.Fatalf("cursor consumed another client's event: %+v", next)
		}
	}
	for range 300 {
		b.Publish("log", "later")
	}
	reset := b.Poll(context.Background(), c)
	if !reset.Reset || reset.Events[0].Name != "state" {
		t.Fatal("slow client did not recover")
	}
	restart := NewBroker().Poll(context.Background(), cursor{Stream: reset.Stream, After: reset.After})
	if !restart.Reset {
		t.Fatal("restart did not invalidate cursor")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	end := b.Poll(ctx, cursor{Stream: reset.Stream, After: reset.After})
	if len(end.Events) != 0 {
		t.Fatal("caught-up cancelled poll replayed events")
	}
}

func TestPrivateSocketOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web.sock")
	l, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	fi, err := os.Stat(path)
	if err != nil || fi.Mode().Perm() != 0600 {
		t.Fatalf("socket is not private: %v %v", fi, err)
	}
	if another, err := Listen(path); err == nil {
		another.Close()
		t.Fatal("replaced a live listener")
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if another, err := Listen(path); err == nil {
		another.Close()
		t.Fatal("overwrote a regular file")
	}
}

func TestDesktopPrivilegesNotExported(t *testing.T) {
	h := NewHandler(nil, NewBroker())
	for _, method := range []string{"App/SetTun", "App/SetSystemProxy", "App/EnableServiceMode", "App/RevealData", "Profiles/ImportFile", "App/InstallUpdate"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/rpc/"+method, strings.NewReader("[]")))
		if w.Code != 404 {
			t.Fatalf("exported %s: %d", method, w.Code)
		}
	}
}

func TestRuntimePreviewRedactsControllerWithoutMovingLines(t *testing.T) {
	input := "mixed-port: 7897\nsecret: private-value\nexternal-controller: 127.0.0.1:45678\nrules:\n    - MATCH,DIRECT\n"
	out := redactController(input)
	if strings.Contains(out, "private-value") || strings.Contains(out, "45678") {
		t.Fatal("controller details escaped into preview")
	}
	if strings.Count(out, "\n") != strings.Count(input, "\n") {
		t.Fatal("redaction moved error line numbers")
	}
	if !strings.Contains(out, "MATCH,DIRECT") {
		t.Fatal("redaction changed routing")
	}
}
