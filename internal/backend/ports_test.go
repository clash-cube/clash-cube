package backend

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/localhost-copilot/clashcube/internal/modules"
	"github.com/localhost-copilot/clashcube/internal/settings"
)

// A port module serves its node whatever the rules say (here REJECT for
// all): a declared node, a provider's (which a listener can't name), and
// a group a route module made, even listed before that module. Every
// proxy and the destination are local fixtures; no external traffic.
func TestPortModule(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a core")
	}
	t.Setenv("CLASHCUBE_HOME", t.TempDir())
	mixed := freePort(t)
	if _, err := settings.Update(func(s *settings.Settings) { s.MixedPort = mixed; s.AutoStart = false }); err != nil {
		t.Fatal(err)
	}
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "served") }))
	defer destination.Close()
	// an HTTP proxy that takes every CONNECT to the destination and says
	// which node it is
	calls := make(chan string, 16)
	node := func(name string) string {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls <- name
			remote, err := net.DialTimeout("tcp", strings.TrimPrefix(destination.URL, "http://"), time.Second)
			if err != nil {
				http.Error(w, err.Error(), 502)
				return
			}
			client, buffered, err := w.(http.Hijacker).Hijack()
			if err != nil {
				remote.Close()
				return
			}
			fmt.Fprint(buffered, "HTTP/1.1 200 Connection Established\r\n\r\n")
			buffered.Flush()
			go func() { defer remote.Close(); io.Copy(remote, buffered) }()
			defer client.Close()
			io.Copy(client, remote)
		}))
		t.Cleanup(s.Close)
		_, port, _ := net.SplitHostPort(strings.TrimPrefix(s.URL, "http://"))
		return port
	}
	profile := fmt.Sprintf(`geodata-mode: false
geo-auto-update: false
proxies:
  - {name: declared, type: http, server: 127.0.0.1, port: %s}
proxy-providers:
  remote:
    type: inline
    payload:
      - {name: 'provided (1)', type: http, server: 127.0.0.1, port: %s}
rules: ['MATCH,REJECT']
`, node("declared"), node("provided"))
	b := New("test", "test", []byte(profile), nopSink{make(chan State, 64)})
	if err := b.Init(); err != nil {
		t.Fatal(err)
	}
	if err := b.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Shutdown)
	id := settings.Load().Profile

	get := func(port int, auth string) (string, error) {
		t.Helper()
		u, _ := url.Parse(fmt.Sprintf("socks5://%s127.0.0.1:%d", auth, port))
		tr := &http.Transport{Proxy: http.ProxyURL(u), DisableKeepAlives: true}
		defer tr.CloseIdleConnections()
		resp, err := (&http.Client{Transport: tr, Timeout: 3 * time.Second}).Get("http://example.test/")
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		return string(body), err
	}
	through := func(port int, auth, want string) {
		t.Helper()
		for len(calls) > 0 {
			<-calls
		}
		body, err := get(port, auth)
		if err != nil || body != "served" {
			t.Fatalf("port %d: %q, %v", port, body, err)
		}
		if got := <-calls; got != want {
			t.Errorf("port %d left by %s, want %s", port, got, want)
		}
	}

	declared, provided, routed := b.FreePort(20000), 0, 0
	provided = b.FreePort(declared + 1)
	routed = b.FreePort(provided + 1)
	route := modules.Route{Service: "Claude", Policy: "select", Pick: true, Nodes: map[string][]string{id: {"provided (1)"}}}
	ms := []modules.Module{
		{Name: "routed", Enabled: true, Port: &modules.Port{Port: routed, Target: map[string]string{id: "Claude"}}},
		{Name: "declared", Enabled: true, Port: &modules.Port{Port: declared, Target: map[string]string{id: "declared"}}},
		{Name: "provided", Enabled: true, Port: &modules.Port{Port: provided, Target: map[string]string{id: "provided (1)"}, User: "u", Pass: "p"}},
		{Name: "Claude", Enabled: true, Route: &route},
	}
	if err := b.SetModules(ms); err != nil {
		t.Fatal(err)
	}
	through(declared, "", "declared")
	through(provided, "u:p@", "provided")
	through(routed, "", "provided")
	if _, err := get(provided, ""); err == nil {
		t.Error("a port with a password let a client in without it")
	}
	if _, err := get(mixed, ""); err == nil {
		t.Error("the mixed port went around REJECT")
	}

	// a node the profile doesn't have: the port refuses rather than going
	// direct
	ms[1].Port.Target[id] = "gone"
	if err := b.SetModules(ms); err != nil {
		t.Fatal(err)
	}
	if _, err := get(declared, ""); err == nil {
		t.Error("a port to a node that is gone let a connection out")
	}

	// the mixed port, a port another app holds, and a port twice
	for _, n := range []int{mixed, holdPort(t)} {
		bad := append([]modules.Module{}, ms...)
		bad[0] = modules.Module{Name: "bad", Enabled: true, Port: &modules.Port{Port: n, Target: map[string]string{id: "declared"}}}
		if err := b.SetModules(bad); err == nil {
			t.Errorf("port %d taken", n)
		}
	}
	twice := append([]modules.Module{}, ms...)
	twice[0] = modules.Module{Name: "twice", Enabled: true, Port: &modules.Port{Port: declared, Target: map[string]string{id: "declared"}}}
	if err := b.SetModules(twice); err == nil {
		t.Error("one port taken twice")
	}
	if got := modules.List(); len(got) != 4 || got[0].Name != "routed" {
		t.Errorf("a refused change was kept: %+v", got)
	}

	// a port another app takes while the core is stopped: the core starts
	// without it, and the state says who holds it; freed, a reload opens it
	waitPorts := func(want map[int]string) {
		t.Helper()
		var got map[int]string
		for i := 0; i < 40; i++ {
			got = map[int]string{}
			for _, p := range b.State().Ports {
				got[p.Port] = p.Status
			}
			if reflect.DeepEqual(got, want) {
				return
			}
			time.Sleep(250 * time.Millisecond)
		}
		t.Fatalf("ports = %v, want %v", got, want)
	}
	waitPorts(map[int]string{routed: "open", declared: "open", provided: "open"})
	if err := b.Stop(); err != nil {
		t.Fatal(err)
	}
	if len(b.State().Ports) != 0 {
		t.Errorf("ports with the core stopped: %v", b.State().Ports)
	}
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", provided))
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Start(); err != nil {
		t.Fatal(err)
	}
	waitPorts(map[int]string{routed: "open", declared: "open", provided: "taken"})
	for _, p := range b.State().Ports {
		if p.Port == provided && p.Holder == "" {
			t.Error("a taken port without its holder")
		}
	}
	if _, err := b.PatchSettings(func(s *settings.Settings) { s.MixedPort = declared }); err == nil || settings.Load().MixedPort != mixed {
		t.Errorf("the mixed port took a port module's port: %v", err)
	}
	l.Close()
	if err := b.Reload(); err != nil {
		t.Fatal(err)
	}
	waitPorts(map[int]string{routed: "open", declared: "open", provided: "open"})
}

// holdPort is a port held by another listener until the test ends.
func holdPort(t *testing.T) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	return l.Addr().(*net.TCPAddr).Port
}

func TestPortStates(t *testing.T) {
	ps := []modules.Port{{Port: 7891}, {Port: 7892}, {Port: 7893}}
	free := func(p int) bool { return p == 7893 }
	holder := func(p int) string { return fmt.Sprint("app", p) }
	got := portStates([]int{9090, 7891}, ps, free, holder)
	want := []PortState{{7891, "open", ""}, {7892, "taken", "app7892"}, {7893, "closed", ""}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v", got)
	}
}
