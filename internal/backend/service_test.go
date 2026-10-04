package backend

import (
	"context"
	"testing"

	"github.com/localhost-copilot/clashcube/internal/settings"
)

func TestHelperUpdatePreservesCoreRunningState(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a core")
	}
	for _, running := range []bool{true, false} {
		name := "stopped"
		if running {
			name = "running"
		}
		t.Run(name, func(t *testing.T) {
			b, _, _, _ := networkBackend(t)
			if running {
				if err := b.Start(); err != nil {
					t.Fatal(err)
				}
			}
			install := helperInstall
			t.Cleanup(func() { helperInstall = install })
			installed := false
			helperInstall = func(data, prompt string) error {
				installed = true
				// Model bootout ending the old core before Install returns.
				// The fake machine leaves the helper unavailable so any new
				// core runs locally, without touching the installed daemon.
				return b.core.Stop()
			}
			if err := b.EnableServiceMode("test"); err != nil {
				t.Fatal(err)
			}
			if !installed || !settings.Load().ServiceMode {
				t.Fatal("service mode was not enabled")
			}
			client := b.core.Client()
			if (client != nil) != running {
				t.Fatalf("core running = %v, want %v", client != nil, running)
			}
			if client != nil {
				if _, err := client.Version(context.Background()); err != nil {
					t.Fatalf("restarted core does not answer: %v", err)
				}
			}
		})
	}
}
