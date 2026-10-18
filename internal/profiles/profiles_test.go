package profiles

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/localhost-copilot/clashcube/internal/appdir"
)

// A chosen agent is what a download sends; one that can't be a header falls
// back to the app's own name instead of failing every update.
func TestUserAgent(t *testing.T) {
	const core, app = "v1.19.0", "0.1.13"
	defaultUA := "clash.meta/v1.19.0 clashcube/0.1.13"
	if got := DefaultUserAgent(core, app); got != defaultUA {
		t.Fatalf("default: %q, want %q", got, defaultUA)
	}
	for _, tc := range []struct {
		custom string
		want   string
	}{
		{"", defaultUA},
		{"   ", defaultUA},
		{" clash-verge/v2.4.2 ", "clash-verge/v2.4.2"},
		{"one\ntwo", defaultUA},
		{"bad\x00agent", defaultUA},
		{strings.Repeat("a", 201), defaultUA},
	} {
		if got := EffectiveUserAgent(core, app, tc.custom); got != tc.want {
			t.Errorf("custom %q: got %q, want %q", tc.custom, got, tc.want)
		}
	}
}

func TestFetchSendsUserAgent(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte("proxies: []\n"))
	}))
	defer srv.Close()
	defer SetUserAgent(UserAgent())
	SetUserAgent(EffectiveUserAgent("v1", "0.1", "clash-verge/v2.4.2"))
	if _, _, err := fetch(srv.URL); err != nil {
		t.Fatal(err)
	}
	if got != "clash-verge/v2.4.2" {
		t.Fatalf("sent %q", got)
	}
}

// A copy is local, with its own file: updating or editing one leaves the
// other be.
func TestDuplicate(t *testing.T) {
	t.Setenv("CLASHCUBE_HOME", t.TempDir())
	if err := appdir.Ensure(); err != nil {
		t.Fatal(err)
	}
	src, err := add(Profile{ID: newID(), Name: "Sub", URL: "https://example.com/sub", Interval: 24}, []byte("proxies: []\nrules: [\"MATCH,DIRECT\"]\n"))
	if err != nil {
		t.Fatal(err)
	}
	cp, err := Duplicate(src.ID, " Sub (copy) ")
	if err != nil {
		t.Fatal(err)
	}
	if cp.ID == src.ID || cp.URL != "" || cp.Interval != 0 || cp.Name != "Sub (copy)" {
		t.Fatalf("copy = %+v", cp)
	}
	if err := os.WriteFile(src.Path(), []byte("rules: [\"MATCH,REJECT\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(cp.Path()); string(b) != "proxies: []\nrules: [\"MATCH,DIRECT\"]\n" {
		t.Errorf("copy's file = %q", b)
	}
	if len(List()) != 2 {
		t.Errorf("profiles = %+v", List())
	}
	if _, err := Duplicate("nope", ""); err == nil {
		t.Error("copied a missing profile")
	}
}
