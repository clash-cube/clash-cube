package settings

import (
	"os"
	"reflect"
	"testing"

	"github.com/localhost-copilot/clashcube/internal/appdir"
)

func TestAICheckPreferences(t *testing.T) {
	t.Setenv("CLASHCUBE_HOME", t.TempDir())
	if err := os.WriteFile(appdir.Settings(), []byte(`{"theme":"dark"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if got := Load(); !got.AIChecks || len(got.AIServices) == 0 {
		t.Fatalf("old settings lost default checks: %+v", got)
	}
	for _, selected := range [][]string{{"Claude"}, {}} {
		if _, err := Update(func(s *Settings) { s.AIChecks = false; s.AIServices = selected }); err != nil {
			t.Fatal(err)
		}
		got := Load()
		if got.AIChecks || !reflect.DeepEqual(got.AIServices, selected) || got.Theme != "dark" {
			t.Fatalf("preferences did not survive reload: %+v", got)
		}
		if _, err := Update(func(s *Settings) { s.AIChecks = true }); err != nil {
			t.Fatal(err)
		}
		if got := Load(); !got.AIChecks || !reflect.DeepEqual(got.AIServices, selected) {
			t.Fatalf("enabling changed selection: %+v", got)
		}
	}
}
