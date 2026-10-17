package core

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/metacubex/chi"
	"github.com/metacubex/http"
	"github.com/metacubex/http/httptest"
	"github.com/metacubex/mihomo/component/tailnet"
)

type testTailnet struct {
	mu     sync.Mutex
	state  string
	logins int
	pings  int
}

func (n *testTailnet) Snapshot() tailnet.Status {
	n.mu.Lock()
	defer n.mu.Unlock()
	return tailnet.Status{State: n.state, Peers: []tailnet.Peer{}}
}
func (*testTailnet) Connect(context.Context) error { return nil }
func (n *testTailnet) Login(context.Context) error {
	n.mu.Lock()
	n.state = "NeedsLogin"
	n.logins++
	n.mu.Unlock()
	tailnet.Notify()
	return nil
}
func (*testTailnet) Logout(context.Context) error { return errors.New("logout refused") }
func (n *testTailnet) Ping(context.Context, netip.Addr) (tailnet.Ping, error) {
	n.mu.Lock()
	n.pings++
	n.mu.Unlock()
	return tailnet.Ping{Milliseconds: 12, Path: "direct"}, nil
}
func (*testTailnet) MatchDomain(string) bool { return false }
func (*testTailnet) MatchIP(netip.Addr) bool { return false }
func (*testTailnet) QueryDNS(context.Context, string, string) ([]byte, error) {
	return nil, errors.New("unused")
}

func TestTailscaleManagementAndStatusStream(t *testing.T) {
	name := "ClashCube Tailscale test-node"
	node := &testTailnet{state: "Starting"}
	tailnet.Replace(map[string]tailnet.Controller{name: node})
	defer tailnet.Replace(nil)
	router := chi.NewRouter()
	registerTailscale(router)
	srv := httptest.NewServer(router)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/clashcube/tailscale?stream=true", nil)
	stream, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	decoder := json.NewDecoder(stream.Body)
	var statuses []tailnet.Status
	if err := decoder.Decode(&statuses); err != nil || len(statuses) != 1 || statuses[0].State != "Starting" {
		t.Fatalf("initial stream: %v %v", statuses, err)
	}
	action := func(action, name, ip string) int {
		t.Helper()
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/clashcube/tailscale/"+action+"?"+url.Values{"name": {name}, "ip": {ip}}.Encode(), nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}
	if code := action("login", name, ""); code != http.StatusOK {
		t.Fatal(code)
	}
	if err := decoder.Decode(&statuses); err != nil || statuses[0].State != "NeedsLogin" {
		t.Fatalf("login event: %v %v", statuses, err)
	}
	if code := action("login", "removed", ""); code != http.StatusNotFound {
		t.Fatal(code)
	}
	if code := action("logout", name, ""); code != http.StatusBadRequest {
		t.Fatal(code)
	}
	if code := action("ping", name, "not-an-address"); code != http.StatusBadRequest {
		t.Fatal(code)
	}
	if code := action("ping", name, "100.64.0.2"); code != http.StatusOK {
		t.Fatal(code)
	}
	if node.pings != 1 || node.logins != 1 {
		t.Fatalf("unexpected operations: %+v", node)
	}
	tailnet.Replace(nil)
	if err := decoder.Decode(&statuses); err != nil || len(statuses) != 0 {
		t.Fatalf("removal event: %v %v", statuses, err)
	}
}
