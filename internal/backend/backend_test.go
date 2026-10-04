package backend

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/localhost-copilot/mihomobar/internal/appdir"
	"github.com/localhost-copilot/mihomobar/internal/core"
	"github.com/localhost-copilot/mihomobar/internal/coremgr"
	"github.com/localhost-copilot/mihomobar/internal/mihomoapi"
	"github.com/localhost-copilot/mihomobar/internal/modules"
	"github.com/localhost-copilot/mihomobar/internal/profiles"
	"github.com/localhost-copilot/mihomobar/internal/settings"
	"github.com/localhost-copilot/mihomobar/internal/userrules"
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
func (nopSink) Latency(LatencySample)       {}

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
	if b, _ := os.ReadFile(appdir.RuntimeConfig()); strings.Contains(string(b), "Nowhere") {
		t.Error("the refused profile was left in the runtime configuration")
	}
	if b.State().Core != "running" {
		t.Error("the core stopped after a failed switch")
	}

	// a setting the core can't take is put back: here the profile in use
	// broke on disk, so the reload it needs fails
	os.WriteFile(p.Path(), []byte("proxies: []\nrules:\n  - MATCH,Nowhere\n"), 0o600)
	level := settings.Load().LogLevel
	if _, err := b.PatchSettings(func(s *settings.Settings) { s.LogLevel = "debug"; s.Theme = "dark" }); err == nil {
		t.Error("a setting the core refused was taken")
	}
	if s := settings.Load(); s.LogLevel != level || s.Theme != "dark" {
		t.Errorf("after a refused reload: logLevel = %q (want %q), theme = %q (want dark)", s.LogLevel, level, s.Theme)
	}
	if b.State().Core != "running" {
		t.Error("the core stopped after a refused setting")
	}
	os.WriteFile(p.Path(), []byte(second), 0o600)

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

// A user rule reaches the running core ahead of the profile's; one the
// core refuses (a GEOIP code it has no database for is fine, so use an
// invalid CIDR) is not kept, and the previous rules stay.
func TestUserRules(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a core")
	}
	t.Setenv("MIHOMOBAR_HOME", t.TempDir())
	if _, err := settings.Update(func(s *settings.Settings) { s.MixedPort = freePort(t); s.AutoStart = false }); err != nil {
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
	c, _ := b.Client()

	if err := b.AddRule(userrules.Rule{Type: "DOMAIN-SUFFIX", Payload: "example.com", Policy: "DIRECT"}); err != nil {
		t.Fatal(err)
	}
	rules, err := c.Rules(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 || rules[0].Type != "DomainSuffix" || rules[0].Payload != "example.com" || rules[0].Proxy != "DIRECT" {
		t.Fatalf("rules = %+v", rules)
	}

	if err := b.AddRule(userrules.Rule{Type: "IP-CIDR", Payload: "not-a-cidr", Policy: "DIRECT"}); err == nil {
		t.Fatal("a broken rule accepted")
	}
	if got := userrules.List(); len(got) != 1 || got[0].Payload != "example.com" {
		t.Errorf("saved rules = %v", got)
	}
	if rules, _ = c.Rules(context.Background()); len(rules) != 2 {
		t.Errorf("core rules = %+v", rules)
	}
}

// A module is taken at once; one the core refuses is not kept, whether the
// core runs or not.
func TestModules(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a core")
	}
	t.Setenv("MIHOMOBAR_HOME", t.TempDir())
	if _, err := settings.Update(func(s *settings.Settings) { s.MixedPort = freePort(t); s.AutoStart = false }); err != nil {
		t.Fatal(err)
	}
	b := New("test", "test", []byte(base), nopSink{make(chan State, 64)})
	if err := b.Init(); err != nil {
		t.Fatal(err)
	}
	good := modules.Module{Name: "lan", Enabled: true, Body: "prepend-rules: [\"DOMAIN-SUFFIX,lan,DIRECT\"]"}
	bad := modules.Module{Name: "bad", Enabled: true, Body: "prepend-rules: [\"DOMAIN,a.com,Nowhere\"]"}
	// stopped: tested, not started
	if err := b.SetModules([]modules.Module{bad}); err == nil {
		t.Fatal("a broken module accepted with the core stopped")
	}
	if len(modules.List()) != 0 {
		t.Fatalf("modules = %+v", modules.List())
	}
	if err := b.SetModules([]modules.Module{good}); err != nil {
		t.Fatal(err)
	}
	if st, _ := b.core.Status(); st == coremgr.Running {
		t.Fatal("checking a module started the core")
	}

	if err := b.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(b.Shutdown)
	c, _ := b.Client()
	rules, err := c.Rules(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 || rules[0].Payload != "lan" {
		t.Fatalf("rules = %+v", rules)
	}
	if err := b.SetModules([]modules.Module{good, bad}); err == nil {
		t.Fatal("a broken module accepted")
	}
	if got := modules.List(); len(got) != 1 || got[0].Name != "lan" {
		t.Errorf("saved modules = %+v", got)
	}
	if b, _ := os.ReadFile(appdir.RuntimeConfig()); strings.Contains(string(b), "Nowhere") {
		t.Error("the refused module was left in the runtime configuration")
	}
	good.Enabled = false
	if err := b.SetModules([]modules.Module{good}); err != nil {
		t.Fatal(err)
	}
	if rules, _ = c.Rules(context.Background()); len(rules) != 1 {
		t.Errorf("core rules = %+v", rules)
	}
}

// Saving the profile in use from an editor reaches the running core; a
// broken save is refused, and the core and runtime.yaml keep what they had.
func TestEditedProfileReloads(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a core")
	}
	t.Setenv("MIHOMOBAR_HOME", t.TempDir())
	if _, err := settings.Update(func(s *settings.Settings) { s.MixedPort = freePort(t); s.AutoStart = false }); err != nil {
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
	c, _ := b.Client()
	p, _ := profiles.Get(settings.Load().Profile)
	rules := func() int { r, _ := c.Rules(context.Background()); return len(r) }

	b.checkProfileFile() // nothing changed
	if n := rules(); n != 1 {
		t.Fatalf("rules = %d", n)
	}
	edited := base + "\n# edited\n"
	edited = strings.Replace(edited, "rules:\n", "rules:\n  - DOMAIN,edited.example,DIRECT\n", 1)
	if err := os.WriteFile(p.Path(), []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	b.checkProfileFile()
	if n := rules(); n != 2 {
		t.Fatalf("after the edit, rules = %d", n)
	}

	if err := os.WriteFile(p.Path(), []byte("rules:\n  - MATCH,Nowhere\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	b.checkProfileFile()
	if n := rules(); n != 2 {
		t.Errorf("after a broken edit, rules = %d", n)
	}
	if rc, _ := os.ReadFile(appdir.RuntimeConfig()); strings.Contains(string(rc), "Nowhere") || !strings.Contains(string(rc), "edited.example") {
		t.Error("runtime.yaml doesn't hold what the core runs")
	}
	evs := b.Events()
	if len(evs) == 0 || evs[len(evs)-1].Level != "error" {
		t.Errorf("events = %+v", evs)
	}
}

// Every ready-made module is one the core takes, on a bare profile. The
// templates that need GEO data download it once, so this is skipped short.
func TestModuleTemplates(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the core's check, which may download GEO data")
	}
	t.Setenv("MIHOMOBAR_HOME", t.TempDir())
	if _, err := settings.Update(func(s *settings.Settings) { s.MixedPort = freePort(t); s.AutoStart = false }); err != nil {
		t.Fatal(err)
	}
	b := New("test", "test", []byte(base), nopSink{make(chan State, 64)})
	if err := b.Init(); err != nil {
		t.Fatal(err)
	}
	for _, tpl := range modules.Templates {
		if err := b.SetModules([]modules.Module{{Name: tpl.Name, Enabled: true, Body: tpl.Body}}); err != nil {
			t.Errorf("%s: %v", tpl.Name, err)
		}
	}
	// every service, each way, over a profile with no nodes at all
	for _, s := range modules.Services {
		for _, r := range []modules.Route{{Service: s.Name, Policy: "select"}, {Service: s.Name, Policy: "url-test", Region: "hk"}, {Service: s.Name, Policy: "select", Pick: true}, {Service: s.Name, Policy: "DIRECT"}} {
			if err := b.SetModules([]modules.Module{{Name: s.Name, Enabled: true, Route: &r}}); err != nil {
				t.Errorf("%+v: %v", r, err)
			}
		}
	}
}

// A route's group takes the profile's nodes by region, and is one to
// choose from like the profile's own.
func TestRouteModule(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a core")
	}
	t.Setenv("MIHOMOBAR_HOME", t.TempDir())
	if _, err := settings.Update(func(s *settings.Settings) { s.MixedPort = freePort(t); s.AutoStart = false }); err != nil {
		t.Fatal(err)
	}
	profile := `
geodata-mode: false
geo-auto-update: false
proxies:
  - { name: "🇭🇰 HK 01", type: ss, server: 192.0.2.1, port: 8388, cipher: aes-128-gcm, password: x }
  - { name: "US 01", type: ss, server: 192.0.2.2, port: 8388, cipher: aes-128-gcm, password: x }
proxy-providers:
  picked:
    type: inline
    payload:
      - { name: "Provider (01)", type: ss, server: 192.0.2.3, port: 8388, cipher: aes-128-gcm, password: x }
proxy-groups:
  - { name: OpenAI, type: select, proxies: [DIRECT] }
rules:
  - MATCH,DIRECT
`
	b := New("test", "test", []byte(profile), nopSink{make(chan State, 64)})
	if err := b.Init(); err != nil {
		t.Fatal(err)
	}
	if err := b.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Shutdown)
	for _, r := range b.RouteRegions() {
		if want := map[string]int{"hk": 1, "us": 1}[r.Key]; r.Count != want {
			t.Errorf("%s: %d nodes, want %d", r.Key, r.Count, want)
		}
	}
	r := modules.Route{Service: "OpenAI", Policy: "select", Region: "us"}
	if err := b.SetModules([]modules.Module{{Name: "OpenAI", Enabled: true, Route: &r}}); err != nil {
		t.Fatal(err)
	}
	c, _ := b.Client()
	all, err := c.Proxies(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	g, ok := all["OpenAI"+modules.Suffix]
	if !ok || len(g.All) != 1 || g.All[0] != "US 01" {
		t.Errorf("group = %+v", g)
	}
	if n := b.Nodes(); len(n) != 3 || n[0].Name != "Provider (01)" {
		t.Errorf("nodes = %+v", n)
	}

	// picked nodes, and ones another profile has: the group refuses
	// rather than going direct
	id := settings.Load().Profile
	r = modules.Route{Service: "Google", Policy: "select", Pick: true, Nodes: map[string][]string{id: {"🇭🇰 HK 01"}, "other": {"US 01"}}}
	gone := modules.Route{Service: "Telegram", Policy: "url-test", Pick: true, Nodes: map[string][]string{"other": {"US 01"}}}
	if err := b.SetModules([]modules.Module{{Name: "Google", Enabled: true, Route: &r}, {Name: "Telegram", Enabled: true, Route: &gone}}); err != nil {
		t.Fatal(err)
	}
	all, _ = c.Proxies(context.Background())
	if g := all["Google"]; len(g.All) != 1 || g.All[0] != "🇭🇰 HK 01" {
		t.Errorf("Google = %+v", g)
	}
	if g := all["Telegram"]; len(g.All) != 1 || g.All[0] != "REJECT" {
		t.Errorf("Telegram = %+v", g)
	}
	// Conversion to editable YAML uses the same concrete-node list as runtime
	// generation, so fixed picks are readable even with a provider present.
	body, err := b.RouteBody(r)
	if err != nil {
		t.Fatal(err)
	}
	v, _ := modules.Parse(body)
	group := v["append-proxy-groups"].([]any)[0].(map[string]any)
	if group["filter"] != nil || !reflect.DeepEqual(group["proxies"], []any{"🇭🇰 HK 01"}) {
		t.Fatalf("editable fixed route = %s", body)
	}
	for _, tc := range []struct{ picks, want []string }{
		{[]string{"🇭🇰 HK 01", "Provider (01)", "gone"}, []string{"Provider (01)", "🇭🇰 HK 01"}},
		{[]string{"Provider (01)"}, []string{"Provider (01)"}},
		{[]string{"gone"}, []string{"REJECT"}},
	} {
		r.Nodes[id] = tc.picks
		if err := b.SetModules([]modules.Module{{Name: "Google", Enabled: true, Route: &r}}); err != nil {
			t.Fatal(err)
		}
		body, err := b.RouteBody(r)
		if err != nil {
			t.Fatal(err)
		}
		// Validate the exported YAML too: provider-only nodes must not become
		// invalid direct references, and an unmatched name must still refuse.
		if err := b.SetModules([]modules.Module{{Name: "Google", Enabled: true, Body: body}}); err != nil {
			t.Fatalf("picks %v: %v", tc.picks, err)
		}
		all, err = c.Proxies(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		got := all["Google"].All
		sort.Strings(got)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("picked group = %v, want %v", got, tc.want)
		}
	}
}

// The core lists a profile's rule providers and takes an update of one.
func TestRuleProviders(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a core")
	}
	t.Setenv("MIHOMOBAR_HOME", t.TempDir())
	if _, err := settings.Update(func(s *settings.Settings) { s.MixedPort = freePort(t); s.AutoStart = false }); err != nil {
		t.Fatal(err)
	}
	profile := base + `rule-providers:
  lan:
    type: inline
    behavior: domain
    payload: ['+.lan', '+.local']
`
	profile = strings.Replace(profile, "rules:\n  - MATCH,Proxy", "rules:\n  - RULE-SET,lan,DIRECT\n  - MATCH,Proxy", 1)
	b := New("test", "test", []byte(profile), nopSink{make(chan State, 64)})
	if err := b.Init(); err != nil {
		t.Fatal(err)
	}
	if err := b.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(b.Shutdown)
	c, _ := b.Client()
	ps, err := c.RuleProviders(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	p, ok := ps["lan"]
	if !ok || p.Behavior != "Domain" || p.VehicleType != "Inline" || p.RuleCount != 2 {
		t.Fatalf("providers = %+v", ps)
	}
	if err := c.UpdateRuleProvider(context.Background(), "lan"); err != nil {
		t.Errorf("update: %v", err)
	}
	if err := c.UpdateRuleProvider(context.Background(), "nope"); err == nil {
		t.Error("an unknown provider updated")
	}
}

// A lookup reports the rule and chain a connection takes, REJECT for a
// rejected one, and a resolver that fails.
func TestLookupHost(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a core")
	}
	t.Setenv("MIHOMOBAR_HOME", t.TempDir())
	if _, err := settings.Update(func(s *settings.Settings) { s.MixedPort = freePort(t); s.AutoStart = false }); err != nil {
		t.Fatal(err)
	}
	// a local server for the DIRECT route to reach
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() { time.Sleep(2 * time.Second); c.Close() }()
		}
	}()
	profile := base + `dns:
  enable: true
  enhanced-mode: fake-ip
  nameserver: [127.0.0.1:1]
hosts:
  direct.test: 127.0.0.1
`
	profile = strings.Replace(profile, "rules:\n  - MATCH,Proxy", "rules:\n  - DOMAIN,blocked.test,REJECT\n  - DOMAIN,direct.test,DIRECT\n  - MATCH,Proxy", 1)
	b := New("test", "test", []byte(profile), nopSink{make(chan State, 64)})
	if err := b.Init(); err != nil {
		t.Fatal(err)
	}
	if err := b.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(b.Shutdown)

	port := l.Addr().(*net.TCPAddr).Port
	r, err := b.LookupHost("direct.test:" + strconv.Itoa(port))
	if err != nil {
		t.Fatal(err)
	}
	if r.Rule != "Domain" || r.RulePayload != "direct.test" || len(r.Chain) != 1 || r.Chain[0] != "DIRECT" {
		t.Errorf("route = %+v", r)
	}
	if r.DNSMode != "fake-ip" {
		t.Errorf("dns mode = %q", r.DNSMode)
	}
	// /dns/query skips hosts and asks the nameserver, which is down
	if r.DNSErr == "" || len(r.A) != 0 {
		t.Errorf("a failing resolver not reported: %+v %q", r.A, r.DNSErr)
	}

	r, err = b.LookupHost("blocked.test")
	if err != nil {
		t.Fatal(err)
	}
	if r.RouteErr != "" || len(r.Chain) != 1 || r.Chain[0] != "REJECT" {
		t.Errorf("rejected = %+v", r)
	}
}
