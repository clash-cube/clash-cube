package backend

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/localhost-copilot/clashcube/internal/appdir"
	"github.com/localhost-copilot/clashcube/internal/profiles"
	"github.com/localhost-copilot/clashcube/internal/settings"
)

func TestProfileEditorAppliesValidEditsAndRollsBackRejectedOnes(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a core")
	}
	t.Setenv("CLASHCUBE_HOME", t.TempDir())
	if _, err := settings.Update(func(s *settings.Settings) { s.MixedPort = freePort(t); s.AutoStart = false }); err != nil {
		t.Fatal(err)
	}
	b := New("test", "test", []byte(base), nopSink{make(chan State, 64)})
	if err := b.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Shutdown)
	if err := b.Start(); err != nil {
		t.Fatal(err)
	}
	id := settings.Load().Profile
	before, _ := profiles.ReadDocument(id)
	changed := strings.Replace(before.Body, "rules:\n", "rules:\n  - DOMAIN,editor.example,DIRECT\n", 1)
	after, err := b.SaveProfileContent(id, before.Revision, changed)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := b.Client()
	rules, err := c.Rules(context.Background())
	if err != nil || len(rules) != 2 {
		t.Fatalf("edited rules did not reach core: %+v, %v", rules, err)
	}
	refused := "rules: [\"MATCH,MissingEditorNode\"]\n"
	if _, err := b.SaveProfileContent(id, after.Revision, refused); err == nil {
		t.Fatal("saved a configuration rejected by the running core")
	}
	if current, _ := profiles.ReadDocument(id); current != after {
		t.Fatal("rejected save changed source or revision")
	}
	runtime, _ := os.ReadFile(appdir.RuntimeConfig())
	if strings.Contains(string(runtime), "MissingEditorNode") || !strings.Contains(string(runtime), "editor.example") {
		t.Fatal("rejected save changed runtime")
	}
	rules, err = c.Rules(context.Background())
	if err != nil || len(rules) != 2 {
		t.Fatal("rejected save changed live rules")
	}
	if err := b.Stop(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.SaveProfileContent(id, after.Revision, refused); err == nil {
		t.Fatal("stopped core accepted an invalid configuration")
	}
	if _, err := b.SaveProfileContent(id, after.Revision, changed+"# saved while stopped\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Client(); err == nil {
		t.Fatal("saving an offline configuration started the core")
	}
}
