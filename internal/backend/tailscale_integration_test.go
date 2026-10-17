//go:build with_gvisor && !no_tailscale

package backend

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/localhost-copilot/clashcube/internal/modules"
	"github.com/localhost-copilot/clashcube/internal/settings"
)

func TestManagedTailscaleCoreReload(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a core")
	}
	control := httptest.NewServer(http.NotFoundHandler())
	defer control.Close()
	t.Setenv("CLASHCUBE_HOME", t.TempDir())
	_, err := settings.Update(func(s *settings.Settings) { s.MixedPort = freePort(t); s.SystemProxy = false; s.AutoStart = false })
	if err != nil {
		t.Fatal(err)
	}
	states := make(chan State, 64)
	b := New("test", "test", []byte(base), nopSink{states})
	if err := b.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Shutdown)
	mod := func(subnet string) modules.Module {
		return modules.Module{Name: "Tailnet", Enabled: true, Body: fmt.Sprintf(`append-proxies:
  - name: private-network
    type: tailscale
    hostname: clashcube-test
    state-dir: tailscale/integration-node
    auto-route: true
    accept-routes: true
    udp: true
    control-url: %q
    route-prefixes: [%s]
append-proxy-groups:
  - {name: automatic, type: url-test, include-all: true, lazy: true}
  - {name: explicit-private, type: select, include-all: true, proxies: [private-network]}
`, control.URL, subnet)}
	}
	if err := b.SetModules([]modules.Module{mod("192.168.50.0/24")}); err != nil {
		t.Fatal(err)
	}
	if err := b.Start(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for {
		select {
		case st := <-states:
			if len(st.Tailscales) == 0 || st.Tailscales[0].State == "Starting" {
				continue
			}
			if st.Tailscales[0].State == "Failed" {
				t.Fatalf("embedded node failed: %s", st.Tailscales[0].Error)
			}
			goto ready
		case <-ctx.Done():
			t.Fatal("embedded node never reported its startup state")
		}
	}
ready:
	c, _ := b.Client()
	proxies, err := c.Proxies(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(proxies["automatic"].All, "private-network") {
		t.Fatal("automatic public group includes private tailnet")
	}
	if !slices.Contains(proxies["explicit-private"].All, "private-network") {
		t.Fatal("removed an explicitly selected private network")
	}
	check := func(target, policy string) {
		t.Helper()
		route, err := c.InspectRoute(ctx, target)
		if err != nil || len(route.Chains) == 0 || route.Chains[len(route.Chains)-1] != policy {
			t.Fatalf("route %s: %+v %v", target, route, err)
		}
	}
	check("192.168.50.2:80", "private-network")
	// A real tsnet session must survive a new config generation without a second
	// state writer; selected subnets are changed without changing its identity.
	if err := b.SetModules([]modules.Module{mod("192.168.60.0/24")}); err != nil {
		t.Fatal(err)
	}
	check("192.168.60.2:80", "private-network")
	if err := b.Reload(); err != nil {
		t.Fatal(err)
	}
	check("192.168.60.2:80", "private-network")
	if err := b.SetModules(nil); err != nil {
		t.Fatal(err)
	}
	for {
		select {
		case st := <-states:
			if len(st.Tailscales) == 0 && st.Core == "running" {
				return
			}
		case <-ctx.Done():
			t.Fatal("disabled tailnet remained in GUI state")
		}
	}
}
