package backend

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
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
	// no test changes the machine's system proxy
	proxySet = func(host string, port int, _ []string) error { fake.set(host, port); return nil }
	proxyClear = func() error { fake.clear(); return nil }
	proxyPointsAt = fake.pointsAt
	proxyEffective = fake.pointsAt
	os.Exit(m.Run())
}

// fakeProxy stands in for the system proxy.
type fakeProxy struct {
	mu      sync.Mutex
	host    string
	port    int
	on      bool
	cleared int
}

var fake = &fakeProxy{}

func (f *fakeProxy) set(host string, port int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.host, f.port, f.on = host, port, true
}

func (f *fakeProxy) clear() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.on = false
	f.cleared++
}

func (f *fakeProxy) pointsAt(host string, port int) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.on && f.host == host && f.port == port
}

func (f *fakeProxy) state() (on bool, cleared int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.on, f.cleared
}

func (f *fakeProxy) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.host, f.port, f.on, f.cleared = "", 0, false, 0
}

// freePort is a local port nothing listens on.
func freePort(t *testing.T) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// A system proxy left at our port by a run that never cleared it (killed,
// crashed) must not outlive this one: it is cleared at start when nothing
// serves the port, and by turning the proxy off or quitting otherwise,
// though this run never set it.
func TestStaleSystemProxy(t *testing.T) {
	newBackend := func(t *testing.T, port int, systemProxy bool) *Backend {
		t.Setenv("MIHOMOBAR_HOME", t.TempDir())
		if _, err := settings.Update(func(s *settings.Settings) {
			s.MixedPort, s.SystemProxy, s.AutoStart = port, systemProxy, false
		}); err != nil {
			t.Fatal(err)
		}
		return New("test", "test", []byte(base), nopSink{make(chan State, 64)})
	}

	t.Run("cleared at start, even when it should be on", func(t *testing.T) {
		fake.reset()
		port := freePort(t)
		fake.set(proxyHost, port)
		b := newBackend(t, port, true)
		if err := b.Init(); err != nil {
			t.Fatal(err)
		}
		if on, _ := fake.state(); on {
			t.Error("a proxy at a port nobody serves was kept")
		}
	})

	t.Run("kept at start while the port is served", func(t *testing.T) {
		fake.reset()
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer l.Close()
		port := l.Addr().(*net.TCPAddr).Port
		fake.set(proxyHost, port)
		b := newBackend(t, port, true)
		if err := b.Init(); err != nil {
			t.Fatal(err)
		}
		if on, _ := fake.state(); !on {
			t.Error("a working proxy was cleared")
		}
		// turning it off clears it, though this run didn't set it
		if err := b.SetSystemProxy(false); err != nil {
			t.Fatal(err)
		}
		if on, _ := fake.state(); on {
			t.Error("turning the system proxy off left it on")
		}
	})

	t.Run("cleared when quitting", func(t *testing.T) {
		fake.reset()
		port := freePort(t)
		b := newBackend(t, port, true)
		if err := b.Init(); err != nil {
			t.Fatal(err)
		}
		fake.set(proxyHost, port)
		b.Shutdown()
		if on, _ := fake.state(); on {
			t.Error("quitting left the proxy on")
		}
	})

	t.Run("someone else's is left alone", func(t *testing.T) {
		fake.reset()
		port := freePort(t)
		b := newBackend(t, port, false)
		if err := b.Init(); err != nil {
			t.Fatal(err)
		}
		fake.set(proxyHost, port+1)
		_ = b.SetSystemProxy(false)
		b.Shutdown()
		if on, cleared := fake.state(); !on || cleared != 0 {
			t.Errorf("a proxy at another port was cleared (on=%v, cleared=%d)", on, cleared)
		}
	})
}

// The proxy the app sets follows the core: on when it starts, off when it
// stops.
func TestSystemProxyFollowsCore(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a core")
	}
	fake.reset()
	t.Setenv("MIHOMOBAR_HOME", t.TempDir())
	port := freePort(t)
	if _, err := settings.Update(func(s *settings.Settings) { s.MixedPort, s.SystemProxy, s.AutoStart = port, true, false }); err != nil {
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
	if !fake.pointsAt(proxyHost, port) {
		t.Fatal("starting the core didn't set the system proxy")
	}
	if err := b.Stop(); err != nil {
		t.Fatal(err)
	}
	if on, _ := fake.state(); on {
		t.Error("stopping the core left the system proxy on")
	}
	if !settings.Load().SystemProxy {
		t.Error("stopping the core forgot the setting")
	}
}

// The proxy latency is taken where the mode and rules send the test URL:
// the policy the MATCH rule names in rule mode, GLOBAL in global mode.
func TestProxyLatencyFollowsRules(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a core")
	}
	fake.reset()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer srv.Close()
	t.Setenv("MIHOMOBAR_HOME", t.TempDir())
	port := freePort(t)
	if _, err := settings.Update(func(s *settings.Settings) {
		s.MixedPort, s.AutoStart, s.Mode, s.TestURL = port, false, "rule", srv.URL+"/generate_204"
	}); err != nil {
		t.Fatal(err)
	}
	// the first group isn't the one the rules use
	const profile = `
geodata-mode: false
geo-auto-update: false
proxies: []
proxy-groups:
  - { name: Side, type: select, proxies: [REJECT] }
  - { name: Main, type: select, proxies: [DIRECT] }
rules:
  - MATCH,Main
`
	b := New("test", "test", []byte(profile), nopSink{make(chan State, 64)})
	if err := b.Init(); err != nil {
		t.Fatal(err)
	}
	if err := b.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(b.Shutdown)

	for _, tc := range []struct{ mode, via string }{{"rule", "Main"}, {"global", "GLOBAL"}} {
		if err := b.SetMode(tc.mode); err != nil {
			t.Fatal(err)
		}
		// only the route: the core's delay test won't time a loopback URL
		c, _ := b.Client()
		_, chain := throughCore(context.Background(), c, http.MethodHead, settings.Load().TestURL)
		if len(chain) == 0 || chain[0] != "DIRECT" || chain[len(chain)-1] != tc.via {
			t.Errorf("%s mode: routed along %q, want DIRECT … %q", tc.mode, chain, tc.via)
		}
	}
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
func (nopSink) Event(Event)                 {}

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
