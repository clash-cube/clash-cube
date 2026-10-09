package coremgr

import (
	"io"
	"os"
	"testing"
	"time"

	"github.com/localhost-copilot/clashcube/internal/runtimecfg"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "core" {
		if len(os.Args) > 2 && os.Args[2] == "-t" {
			os.Exit(0)
		}
		_, _ = os.Stdout.WriteString("ready\n")
		_, _ = io.Copy(io.Discard, os.Stdin)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestLocalRunnerLifetimePipe(t *testing.T) {
	for _, stop := range []bool{true, false} {
		t.Run(map[bool]string{true: "stop", false: "parent-disconnect"}[stop], func(t *testing.T) {
			r := &LocalRunner{}
			ready := make(chan struct{}, 1)
			done, err := r.Start(t.TempDir(), "unused.yaml", runtimecfg.Controller{}, func(line string) {
				if line == "ready" {
					ready <- struct{}{}
				}
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = r.Stop() })
			select {
			case <-ready:
			case <-time.After(5 * time.Second):
				t.Fatal("child did not start")
			}
			if stop {
				err = r.Stop()
			} else {
				err = r.keep.Close()
			}
			if err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("child did not exit gracefully: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("child survived lifetime pipe close")
			}
		})
	}
}
