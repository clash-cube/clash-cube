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
	if got := Load(); len(got.AIServices) == 0 {
		t.Fatalf("old settings lost default checks: %+v", got)
	}
	for _, selected := range [][]string{{"Claude"}, {}} {
		if _, err := Update(func(s *Settings) { s.AIServices = selected }); err != nil {
			t.Fatal(err)
		}
		if got := Load(); !reflect.DeepEqual(got.AIServices, selected) || got.Theme != "dark" {
			t.Fatalf("preferences did not survive reload: %+v", got)
		}
	}
}

// The switch that used to turn every check off now clears the selection;
// on, it leaves the selection as it was.
func TestAIChecksSwitchMigrates(t *testing.T) {
	t.Setenv("CLASHCUBE_HOME", t.TempDir())
	for _, tc := range []struct {
		file string
		want []string
	}{
		{`{"aiChecks":false,"aiServices":["Claude"]}`, []string{}},
		{`{"aiChecks":true,"aiServices":["Claude"]}`, []string{"Claude"}},
	} {
		if err := os.WriteFile(appdir.Settings(), []byte(tc.file), 0600); err != nil {
			t.Fatal(err)
		}
		if got := Load().AIServices; !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %q, want %q", tc.file, got, tc.want)
		}
	}
	if _, err := Update(func(s *Settings) { s.AIServices = []string{"OpenAI"} }); err != nil {
		t.Fatal(err)
	}
	if got := Load().AIServices; !reflect.DeepEqual(got, []string{"OpenAI"}) {
		t.Errorf("a selection after migrating was overridden: %q", got)
	}
}
