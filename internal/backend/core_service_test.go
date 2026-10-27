package backend

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/localhost-copilot/clashcube/internal/appdir"
	"github.com/localhost-copilot/clashcube/internal/runtimecfg"
)

type observedRunner struct{ starts, stops int }

func (r *observedRunner) Start(_, _ string, _ runtimecfg.Controller, _ func(string)) (<-chan error, error) {
	r.starts++
	return nil, fmt.Errorf("attachment must not start a process")
}
func (r *observedRunner) Stop() error            { r.stops++; return nil }
func (r *observedRunner) Test(_, _ string) error { return nil }

func TestExplicitStopReachesServiceBeforeCoreIsRunning(t *testing.T) {
	t.Setenv("CLASHCUBE_HOME", t.TempDir())
	runner := &observedRunner{}
	b := New("test", "test", []byte(base), nil)
	b.CoreService = runner
	if err := b.Init(); err != nil {
		t.Fatal(err)
	}
	defer b.Shutdown()
	if err := b.Stop(); err != nil {
		t.Fatal(err)
	}
	if runner.stops != 1 {
		t.Fatal("explicit Stop did not cancel the supervisor's pending restore")
	}
}

func TestCoreServiceAttachmentAndManagementShutdown(t *testing.T) {
	for _, stop := range []bool{false, true} {
		t.Run(fmt.Sprintf("explicit-stop=%v", stop), func(t *testing.T) {
			t.Setenv("CLASHCUBE_HOME", t.TempDir())
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/version" {
					fmt.Fprint(w, `{"version":"service"}`)
				} else {
					fmt.Fprint(w, `{"ports":[]}`)
				}
			}))
			defer api.Close()
			runner := &observedRunner{}
			b := New("test", "test", []byte(base), nil)
			b.CoreService = runner
			if err := b.Init(); err != nil {
				t.Fatal(err)
			}
			defer b.Shutdown()
			const runtime = "existing runtime and controller must survive attachment\n"
			if err := os.WriteFile(appdir.RuntimeConfig(), []byte(runtime), 0600); err != nil {
				t.Fatal(err)
			}
			exited := make(chan error, 1)
			defer close(exited)
			ctl := runtimecfg.Controller{Addr: strings.TrimPrefix(api.URL, "http://"), Secret: "private"}
			if err := b.AttachCore(ctl, exited); err != nil {
				t.Fatal(err)
			}
			b.Boot()
			if err := b.Start(); err != nil {
				t.Fatal(err)
			}
			if runner.starts != 0 {
				t.Fatal("management reconnected by starting a core")
			}
			b.ObserveCoreVersion("independently-upgraded")
			if b.State().CoreVersion != "independently-upgraded" || !strings.Contains(b.DefaultUserAgent(), "independently-upgraded") {
				t.Fatal("independent core upgrade left stale version or subscription identification")
			}
			body, err := os.ReadFile(appdir.RuntimeConfig())
			if err != nil || string(body) != runtime {
				t.Fatal("attachment or repeated Start rewrote the runtime")
			}
			if stop {
				if err := b.Stop(); err != nil {
					t.Fatal(err)
				}
			}
			b.Shutdown()
			want := 0
			if stop {
				want = 1
			}
			if runner.stops != want {
				t.Fatalf("service stops = %d, want %d", runner.stops, want)
			}
		})
	}
}
