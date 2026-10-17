package backend

import (
	"net/http"
	"testing"

	"github.com/localhost-copilot/clashcube/internal/modules"
	"github.com/localhost-copilot/clashcube/internal/settings"
)

func TestTailscaleAPIRequiresCoreSecret(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a core")
	}
	t.Setenv("CLASHCUBE_HOME", t.TempDir())
	_, err := settings.Update(func(s *settings.Settings) { s.MixedPort = freePort(t); s.SystemProxy = false; s.AutoStart = false })
	if err != nil {
		t.Fatal(err)
	}
	b := New("test", "test", []byte(base), nil)
	if err := b.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Shutdown)
	if err := b.Start(); err != nil {
		t.Fatal(err)
	}
	ctl := b.core.Controller()
	for _, path := range []string{"/clashcube/tailscale", "/clashcube/tailscale?stream=true", "/clashcube/tailscale/login?name=missing"} {
		method := http.MethodGet
		if path == "/clashcube/tailscale/login?name=missing" {
			method = http.MethodPost
		}
		req, _ := http.NewRequest(method, "http://"+ctl.Addr+path, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("unauthorized %s: %d", path, resp.StatusCode)
		}
	}
	if err := b.TailscaleLogin("missing"); err == nil {
		t.Fatal("login to a missing connection succeeded")
	}
}

func TestTailscaleReconnectsOnlyForConnectionSettings(t *testing.T) {
	ts := func(host, control string, routes ...string) modules.Module {
		return modules.Module{Enabled: true, Tailscale: &modules.Tailscale{Identity: "identity-001", Hostname: host, ControlURL: control, Routes: routes}}
	}
	before := []modules.Module{ts("mac", "")}
	for _, c := range []struct {
		name string
		ms   []modules.Module
		want bool
	}{
		{"subnets", []modules.Module{ts("mac", "", "192.168.50.0/24")}, false},
		{"hostname", []modules.Module{ts("studio", "")}, true},
		{"control server", []modules.Module{ts("mac", "https://hs.example.com")}, true},
		{"disabled", []modules.Module{{Tailscale: &modules.Tailscale{Identity: "identity-001", Hostname: "studio"}}}, false},
	} {
		if got := tailscaleReconnects(before, c.ms); got != c.want {
			t.Errorf("%s: reconnects %v, want %v", c.name, got, c.want)
		}
	}
}
