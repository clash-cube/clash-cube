package appdir

import (
	"path/filepath"
	"testing"
)

func TestVolatileCoreDataKeepsConfigurationPersistent(t *testing.T) {
	persistent, volatile := t.TempDir(), t.TempDir()
	t.Setenv("CLASHCUBE_HOME", persistent)
	t.Setenv("CLASHCUBE_CORE_HOME", filepath.Join(volatile, "core"))
	t.Setenv("CLASHCUBE_CACHE_HOME", volatile)
	if err := Ensure(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{Settings(), Profiles(), RuntimeConfig()} {
		rel, err := filepath.Rel(persistent, path)
		if err != nil || !filepath.IsLocal(rel) {
			t.Fatalf("configuration escaped persistent storage: %s", path)
		}
	}
	for _, path := range []string{CoreHome(), Logs(), Usage()} {
		rel, err := filepath.Rel(volatile, path)
		if err != nil || !filepath.IsLocal(rel) {
			t.Fatalf("cache escaped volatile storage: %s", path)
		}
	}
}
