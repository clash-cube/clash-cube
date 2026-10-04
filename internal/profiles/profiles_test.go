package profiles

import (
	"os"
	"testing"

	"github.com/localhost-copilot/mihomobar/internal/appdir"
)

// A copy is local, with its own file: updating or editing one leaves the
// other be.
func TestDuplicate(t *testing.T) {
	t.Setenv("MIHOMOBAR_HOME", t.TempDir())
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
