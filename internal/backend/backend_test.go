package backend

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/localhost-copilot/mihomobar/internal/core"
	"github.com/localhost-copilot/mihomobar/internal/mihomoapi"
	"github.com/localhost-copilot/mihomobar/internal/profiles"
	"github.com/localhost-copilot/mihomobar/internal/settings"
)

// The test binary is its own core: LocalRunner starts os.Executable() with
// "core", which lands here.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "core" {
		if err := core.Main(os.Args[2:]); err != nil {
			os.Stderr.WriteString(err.Error() + "\n")
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

const base = `
mixed-port: 7890
geodata-mode: false
geo-auto-update: false
proxies: []
proxy-groups:
  - { name: Proxy, type: select, proxies: [DIRECT] }
rules:
  - MATCH,Proxy
`

const second = `
geodata-mode: false
geo-auto-update: false
proxies:
  - { name: hk, type: ss, server: 192.0.2.1, port: 8388, cipher: aes-128-gcm, password: x }
proxy-groups:
  - { name: Choose, type: select, proxies: [DIRECT, hk] }
rules:
  - MATCH,Choose
`

type nopSink struct{ states chan State }

func (s nopSink) State(st State) {
	select {
	case s.states <- st:
	default:
	}
}
func (nopSink) Traffic(mihomoapi.Traffic)   {}
func (nopSink) Memory(mihomoapi.Memory)     {}
func (nopSink) Log(mihomoapi.Log)           {}
func (nopSink) Profiles([]profiles.Profile) {}

func TestCoreLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a core")
	}
	t.Setenv("MIHOMOBAR_HOME", t.TempDir())
	if _, err := settings.Update(func(s *settings.Settings) { s.MixedPort = 17899; s.AutoStart = false }); err != nil {
		t.Fatal(err)
	}
	b := New("test", "test", []byte(base), nopSink{make(chan State, 64)})
	if err := b.Init(); err != nil {
		t.Fatal(err)
	}
	if err := b.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(b.Shutdown)
	if st := b.State(); st.Core != "running" {
		t.Fatalf("state = %+v", st)
	}
	ctx := context.Background()
	c, _ := b.Client()

	// the app's settings are laid over the profile
	cfg, err := c.Configs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MixedPort != 17899 {
		t.Errorf("mixed-port = %d, want 17899", cfg.MixedPort)
	}

	if err := b.SetMode("global"); err != nil {
		t.Fatal(err)
	}
	if cfg, _ = c.Configs(ctx); cfg.Mode != "global" {
		t.Errorf("mode = %q after SetMode", cfg.Mode)
	}
	if settings.Load().Mode != "global" {
		t.Error("mode not saved")
	}

	// a profile switch reloads the running core in place
	f := t.TempDir() + "/second.yaml"
	os.WriteFile(f, []byte(second), 0o600)
	p, err := profiles.ImportFile(f, "Second")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.UseProfile(p.ID); err != nil {
		t.Fatalf("use profile: %v", err)
	}
	all, err := c.Proxies(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := all["Choose"]; !ok {
		t.Fatalf("reloaded core has no Choose group: %v", keys(all))
	}
	if cfg, _ = c.Configs(ctx); cfg.Mode != "global" || cfg.MixedPort != 17899 {
		t.Errorf("reload lost the overlay: %+v", cfg)
	}
	if err := c.Select(ctx, "Choose", "hk"); err != nil {
		t.Fatal(err)
	}
	if all, _ = c.Proxies(ctx); all["Choose"].Now != "hk" {
		t.Errorf("Choose.now = %q", all["Choose"].Now)
	}

	// a broken profile is refused and the old one stays
	bad := t.TempDir() + "/bad.yaml"
	os.WriteFile(bad, []byte("proxies: []\nrules:\n  - MATCH,Nowhere\n"), 0o600)
	pb, err := profiles.ImportFile(bad, "Bad")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.UseProfile(pb.ID); err == nil {
		t.Error("a profile naming a missing policy was taken")
	}
	if settings.Load().Profile != p.ID {
		t.Error("the failed switch was kept")
	}
	if b.State().Core != "running" {
		t.Error("the core stopped after a failed switch")
	}

	if err := b.Restart(); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if b.State().Core != "running" {
		t.Fatal("not running after restart")
	}
	if err := b.Stop(); err != nil {
		t.Fatal(err)
	}
	if b.State().Core != "stopped" {
		t.Errorf("state after stop = %s", b.State().Core)
	}
	time.Sleep(100 * time.Millisecond)
	if _, err := c.Version(ctx); err == nil {
		t.Error("core still answers after stop")
	}
}

func keys[V any](m map[string]V) []string {
	var k []string
	for n := range m {
		k = append(k, n)
	}
	return k
}
