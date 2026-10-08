package backend

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/localhost-copilot/clashcube/internal/modules"
	"github.com/localhost-copilot/clashcube/internal/runtimecfg"
	"github.com/localhost-copilot/clashcube/internal/settings"
)

// A real core must connect through the front before it can reach the exit.
// Both proxies and the destination are local fixtures; no external traffic.
func TestRouteUpstream(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a core")
	}
	t.Setenv("CLASHCUBE_HOME", t.TempDir())
	port := freePort(t)
	if _, err := settings.Update(func(s *settings.Settings) { s.MixedPort = port; s.AutoStart = false }); err != nil {
		t.Fatal(err)
	}
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "through both proxies") }))
	defer destination.Close()
	tunnel := func(w http.ResponseWriter, r *http.Request, target string) {
		if r.Method != http.MethodConnect {
			http.Error(w, "CONNECT required", 400)
			return
		}
		remote, err := net.DialTimeout("tcp", target, time.Second)
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
	}
	exitCalls := make(chan string, 16)
	exit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		exitCalls <- r.Host
		tunnel(w, r, strings.TrimPrefix(destination.URL, "http://"))
	}))
	defer exit.Close()
	frontCalls := make(chan string, 16)
	front := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		frontCalls <- r.Host
		tunnel(w, r, strings.TrimPrefix(exit.URL, "http://"))
	}))
	defer front.Close()
	_, frontPort, _ := net.SplitHostPort(strings.TrimPrefix(front.URL, "http://"))
	_, exitPort, _ := net.SplitHostPort(strings.TrimPrefix(exit.URL, "http://"))
	profile := fmt.Sprintf(`geodata-mode: false
geo-auto-update: false
proxies:
  - {name: front, type: http, server: 127.0.0.1, port: %s}
proxy-providers:
  exits:
    type: inline
    payload:
      - {name: 'exit^(1)', type: http, server: 127.0.0.1, port: %s}
    override:
      additional-prefix: 'US '
      override-expr: ['.name += " renamed"']
proxy-groups:
  - {name: original, type: select, include-all: true}
rules: ['MATCH,REJECT']
`, frontPort, exitPort)
	b := New("test", "test", []byte(profile), nopSink{make(chan State, 64)})
	if err := b.Init(); err != nil {
		t.Fatal(err)
	}
	if err := b.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Shutdown)
	id := settings.Load().Profile
	name := "US exit^(1) renamed"
	r := modules.Route{Service: "Claude", Policy: "select", Pick: true, Nodes: map[string][]string{id: {name}}, Upstream: map[string]string{id: "front"}}
	setRoute := func() {
		t.Helper()
		if err := b.SetModules([]modules.Module{{Name: "Claude", Enabled: true, Route: &r}}); err != nil {
			t.Fatal(err)
		}
	}
	setRoute()
	c, _ := b.Client()
	all, err := c.Proxies(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if g := all["Claude"]; len(g.All) != 1 || runtimecfg.NodeLabel(g.All[0]) != name {
		t.Fatalf("route group = %+v", g)
	}
	for _, member := range all["original"].All {
		if strings.HasPrefix(member, runtimecfg.ChainPrefix) {
			t.Fatalf("original group includes chain: %v", all["original"].All)
		}
	}
	if nodes := b.Nodes(); len(nodes) != 2 {
		t.Fatalf("picker exposes private copies: %+v", nodes)
	}
	proxyURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	transport := &http.Transport{Proxy: http.ProxyURL(proxyURL), DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	resp, err := client.Get("http://claude.ai/")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || string(body) != "through both proxies" {
		t.Fatalf("response %q, %v", body, err)
	}
	select {
	case host := <-frontCalls:
		if host != "127.0.0.1:"+exitPort {
			t.Errorf("front dialed %s", host)
		}
	default:
		t.Fatal("request bypassed front")
	}
	select {
	case host := <-exitCalls:
		if host != "claude.ai:80" {
			t.Errorf("exit dialed %s", host)
		}
	default:
		t.Fatal("request bypassed exit")
	}
	// Exporting to YAML must preserve the same chain and remain loadable.
	exported, err := b.ModuleBody(modules.Module{Route: &r})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.SetModules([]modules.Module{{Name: "Claude", Enabled: true, Body: exported}}); err != nil {
		t.Fatal(err)
	}
	// Empty picks must not accidentally admit every provider node.
	r.Nodes[id] = nil
	setRoute()
	all, _ = c.Proxies(context.Background())
	if g := all["Claude"]; len(g.All) != 1 || g.All[0] != "REJECT" {
		t.Fatalf("empty picks = %+v", g)
	}
	// A missing front fails closed even while the exit remains available.
	r.Nodes[id] = []string{name}
	r.Upstream[id] = "removed front"
	setRoute()
	resp, err = client.Get("http://claude.ai/")
	if err == nil {
		body, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
		if string(body) == "through both proxies" {
			t.Fatal("missing front was bypassed")
		}
	}
	select {
	case <-frontCalls:
		t.Fatal("used old front after removal")
	default:
	}
	select {
	case <-exitCalls:
		t.Fatal("dialed exit without front")
	default:
	}
}
