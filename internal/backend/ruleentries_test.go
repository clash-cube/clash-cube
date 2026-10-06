package backend

import (
	"path/filepath"
	"testing"

	"github.com/localhost-copilot/clashcube/internal/appdir"
)

func TestProviderPathStaysInCoreHome(t *testing.T) {
	t.Setenv("CLASHCUBE_HOME", t.TempDir())
	home := appdir.CoreHome()
	for _, c := range []struct {
		p    providerSchema
		want string
	}{
		{providerSchema{Type: "http", Path: "./ruleset/a.mrs"}, filepath.Join(home, "ruleset", "a.mrs")},
		{providerSchema{Type: "http", URL: "https://example.com/a"}, filepath.Join(home, "rules", "8a8e1ff70bb6c7348cdbb4d5f2b1b1f0")},
		{providerSchema{Type: "file", Path: filepath.Join(home, "b.yaml")}, filepath.Join(home, "b.yaml")},
	} {
		got, err := providerPath(c.p)
		if err != nil || filepath.Dir(got) != filepath.Dir(c.want) {
			t.Errorf("%+v: %q %v", c.p, got, err)
		}
	}
	for _, p := range []providerSchema{
		{Type: "file", Path: "../settings.json"},
		{Type: "file", Path: "/etc/hosts"},
		{Type: "file"},
	} {
		if got, err := providerPath(p); err == nil {
			t.Errorf("%+v accepted as %q", p, got)
		}
	}
}

func TestParseRuleSet(t *testing.T) {
	got, err := parseRuleSet([]byte("payload:\n  - '+.example.com'\n  - DOMAIN,a.example\n"), "classical", "yaml")
	if err != nil || len(got) != 2 || got[0] != "+.example.com" {
		t.Errorf("yaml: %q %v", got, err)
	}
	got, err = parseRuleSet([]byte("# c\n10.0.0.0/8\n\n// c\n192.168.0.0/16\n"), "ipcidr", "text")
	if err != nil || len(got) != 2 || got[1] != "192.168.0.0/16" {
		t.Errorf("text: %q %v", got, err)
	}
}
