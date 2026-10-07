package backend

import (
	"fmt"
	"testing"
	"time"

	"github.com/localhost-copilot/clashcube/internal/mihomoapi"
)

func TestNodeCountry(t *testing.T) {
	for name, want := range map[string]string{
		"🇯🇵 Tokyo 01":                         "JP",
		"Relay 🇩🇪 Frankfurt":                  "DE",
		"香港 IPLC 02":                          "HK",
		"US-LA 03":                            "US",
		"ClashCube chain/hk-route/nodes/SG 1": "SG", // the route's name isn't the node's
		"Premium 01":                          "",
		"DIRECT":                              "",
	} {
		if got := nodeCountry(name); got != want {
			t.Errorf("nodeCountry(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestGlobeRoutes(t *testing.T) {
	conn := func(id, host, dest string, up, down int64, chains ...string) mihomoapi.Connection {
		return mihomoapi.Connection{ID: id, Metadata: mihomoapi.Metadata{Host: host, DestIP: dest}, Upload: up, Download: down, Chains: chains}
	}
	inner := conn("10", "", "JP", 5, 5, "🇭🇰 HK 01", "Front") // a node dialling its server through its front node
	inner.Metadata.Type = "Inner"
	update := conn("12", "rules.example", "US", 5, 5, "🇭🇰 HK 01", "Proxy") // the core fetching a rule set
	update.Metadata.Type = "Inner"
	conns := []mihomoapi.Connection{
		conn("1", "a.example", "US", 10, 100, "🇭🇰 HK 01", "Proxy"),
		conn("2", "b.example", "US", 1, 1000, "🇭🇰 HK 02", "Proxy"),
		conn("3", "c.example", "US", 5, 5, "DIRECT"),
		conn("4", "d.example", "CN", 5, 5, "DIRECT"),
		conn("5", "e.example", "US", 5, 5, "REJECT"),
		conn("6", "f.example", "", 5, 5, "DIRECT"),   // no country
		conn("7", "g.example", "ZZ", 5, 5, "DIRECT"), // no centre
		conn("8", "h.example", "JP", 5, 5, "Premium 01", "Proxy"),
		conn("9", "", "JP", 5, 5),
		conn("11", "i.example", "US", 5, 5, "🇯🇵 JP 01", "Proxy"), // through a front node in HK
		inner, update,
	}
	rates := map[string][2]int64{"1": {1, 2}, "2": {3, 4000}, "4": {0, 10}}
	to := func(c mihomoapi.Connection) (string, string) { return c.Metadata.DestIP, c.Metadata.DestIP }
	via := func(node string) []string {
		if node == "🇯🇵 JP 01" {
			return []string{"HK", "JP"}
		}
		if cc := nodeCountry(node); cc != "" {
			return []string{cc}
		}
		return nil
	}
	routes := globeRoutes(conns, rates, to, via, map[string]bool{"Front": true})

	want := []GlobeRoute{
		{To: "US", Via: []string{"HK"}, Conns: 3, Up: 4, Down: 4002, Total: 1121},
		{To: "CN", Direct: true, Conns: 1, Down: 10, Total: 10},
		{To: "US", Direct: true, Conns: 1, Total: 10},
		{To: "JP", Conns: 1, Total: 10},
		{To: "US", Via: []string{"HK", "JP"}, Conns: 1, Total: 10},
	}
	if len(routes) != len(want) {
		t.Fatalf("routes = %+v", routes)
	}
	for i, w := range want {
		r := routes[i]
		if r.To != w.To || fmt.Sprint(r.Via) != fmt.Sprint(w.Via) || r.Direct != w.Direct || r.Conns != w.Conns || r.Up != w.Up || r.Down != w.Down || r.Total != w.Total {
			t.Errorf("route %d = %+v, want %+v", i, r, w)
		}
	}
	if h := routes[0].Hosts; len(h) != 3 || h[0].Host != "b.example" || h[0].Total != 1001 {
		t.Errorf("hosts = %+v, want b.example first", h)
	}
	if e := routes[0].Ends; len(e) != 1 || e[0].At != "US" || e[0].Conns != 3 || len(e[0].Hosts) != 3 {
		t.Errorf("ends = %+v", e)
	}
}

func TestGlobeRouteEnds(t *testing.T) {
	conn := func(id, ip string, down int64) mihomoapi.Connection {
		return mihomoapi.Connection{ID: id, Metadata: mihomoapi.Metadata{Host: id + ".example", DestIP: ip}, Download: down, Chains: []string{"DIRECT"}}
	}
	cities := map[string]string{"1": "US/Ashburn", "2": "US/San Jose", "3": "US/Ashburn"}
	to := func(c mihomoapi.Connection) (string, string) { return cities[c.ID], c.Metadata.DestIP }
	routes := globeRoutes([]mihomoapi.Connection{conn("1", "US", 10), conn("2", "US", 50), conn("3", "US", 20)}, nil, to, nil, nil)
	if len(routes) != 1 {
		t.Fatalf("a country's cities split its route: %+v", routes)
	}
	e := routes[0].Ends
	if len(e) != 2 || e[0].At != "US/San Jose" || e[1].At != "US/Ashburn" || e[1].Conns != 2 || e[1].Total != 30 {
		t.Errorf("ends = %+v", e)
	}
}

func TestGlobeChain(t *testing.T) {
	g := globeState{proxies: map[string]mihomoapi.Proxy{
		"exit":    {Name: "exit", Dialer: "front"},
		"front":   {Name: "front", All: []string{"hk", "sg"}, Now: "hk"},
		"hk":      {Name: "hk", Dialer: "entry"},
		"entry":   {Name: "entry"},
		"loop-a":  {Name: "loop-a", Dialer: "loop-b"},
		"loop-b":  {Name: "loop-b", Dialer: "loop-a"},
		"to-none": {Name: "to-none", Dialer: "empty"},
		"empty":   {Name: "empty", All: []string{"x"}},
	}}
	for exit, want := range map[string][]string{
		"exit":    {"entry", "hk", "exit"},
		"entry":   {"entry"},
		"loop-a":  {"loop-b", "loop-a"},
		"to-none": {"to-none"},
		"unknown": {"unknown"},
	} {
		if got := g.chain(exit); fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("chain(%s) = %v, want %v", exit, got, want)
		}
	}
}

func TestGlobeSample(t *testing.T) {
	var g globeState
	t0 := time.Now()
	c := mihomoapi.Connection{ID: "a", Upload: 100, Download: 1000, Start: t0.Add(-time.Minute)}
	if r := g.sample(t0, []mihomoapi.Connection{c}); len(r) != 0 {
		t.Fatalf("first sample has speeds: %v", r)
	}
	c.Upload, c.Download = 300, 3000
	late := mihomoapi.Connection{ID: "b", Upload: 50, Download: 50, Start: t0.Add(time.Second)}
	old := mihomoapi.Connection{ID: "c", Upload: 50, Download: 50, Start: t0.Add(-time.Second)} // unseen, so its bytes predate the last call
	r := g.sample(t0.Add(2*time.Second), []mihomoapi.Connection{c, late, old})
	if r["a"] != [2]int64{100, 1000} || r["b"] != [2]int64{25, 25} || r["c"] != [2]int64{} {
		t.Errorf("rates = %v", r)
	}
	c.Upload += 10
	if r := g.sample(t0.Add(2100*time.Millisecond), []mihomoapi.Connection{c}); r["a"] != [2]int64{100, 1000} {
		t.Errorf("a call soon after gave %v, not the last measure", r)
	}
	// later measures go back about speedWindow, not to the last call
	c.Upload = 610
	g.sample(t0.Add(3*time.Second), []mihomoapi.Connection{c})
	c.Upload = 910
	if r := g.sample(t0.Add(5*time.Second), []mihomoapi.Connection{c}); r["a"][0] != (910-300)/3 {
		t.Errorf("upload = %d, want it measured from 2s", r["a"][0])
	}
	if r := g.sample(t0.Add(2*time.Minute), []mihomoapi.Connection{c}); len(r) != 0 {
		t.Errorf("a sample after a pause has speeds: %v", r)
	}
}

func TestGlobeAddr(t *testing.T) {
	var g globeState
	g.hosts = map[string]hostAddr{"cached.example": {"9.9.9.9", time.Now()}}
	for _, tc := range []struct {
		md     mihomoapi.Metadata
		chains []string
		want   string
	}{
		{mihomoapi.Metadata{DestIP: "1.1.1.1", Host: "x.example"}, []string{"Proxy"}, "1.1.1.1"},
		{mihomoapi.Metadata{DestIP: "198.18.0.5", RemoteDest: "8.8.8.8", Host: "x.example"}, []string{"DIRECT"}, "8.8.8.8"},
		{mihomoapi.Metadata{RemoteDest: "5.5.5.5:443", Host: "cached.example"}, []string{"node"}, "9.9.9.9"}, // a proxy's server isn't the destination
		{mihomoapi.Metadata{SniffHost: "Cached.Example."}, []string{"node"}, "9.9.9.9"},
	} {
		if got := g.addrOf(nil, mihomoapi.Connection{Metadata: tc.md, Chains: tc.chains}); got != tc.want {
			t.Errorf("addrOf(%+v) = %q, want %q", tc.md, got, tc.want)
		}
	}
}
