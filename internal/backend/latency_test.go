package backend

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/localhost-copilot/mihomobar/internal/mihomoapi"
)

func latencyCore(t *testing.T, proxies map[string]mihomoapi.Proxy, providers map[string]mihomoapi.ProxyProvider, probe http.HandlerFunc) *mihomoapi.Client {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/proxies":
			_ = json.NewEncoder(w).Encode(map[string]any{"proxies": proxies})
		case "/providers/proxies":
			_ = json.NewEncoder(w).Encode(map[string]any{"providers": providers})
		default:
			probe(w, r)
		}
	}))
	t.Cleanup(s.Close)
	return mihomoapi.New(strings.TrimPrefix(s.URL, "http://"), "")
}

func TestLatencyAllDeduplicatesAndUsesProviders(t *testing.T) {
	proxies := map[string]mihomoapi.Proxy{
		"first":  {Name: "first", Type: "Selector", All: []string{"shared", "sub"}, Now: "shared"},
		"second": {Name: "second", Type: "Selector", All: []string{"shared"}, Now: "shared"},
		"shared": {Name: "shared", Type: "Shadowsocks"},
		"reject": {Name: "reject", Type: "Reject"},
		"drop":   {Name: "drop", Type: "RejectDrop"},
		"block":  {Name: "block", Type: "Block"},
	}
	providers := map[string]mihomoapi.ProxyProvider{
		"subscription": {VehicleType: "HTTP", Proxies: []mihomoapi.Proxy{{Name: "sub", Type: "VMess"}, proxies["shared"]}},
	}
	var mu sync.Mutex
	paths := map[string]int{}
	c := latencyCore(t, proxies, providers, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("url") != "https://test.invalid/204" || r.URL.Query().Get("timeout") != "2000" {
			t.Errorf("unexpected probe parameters: %s", r.URL.RawQuery)
		}
		mu.Lock()
		paths[r.URL.Path]++
		mu.Unlock()
		if strings.Contains(r.URL.Path, "/sub/") {
			http.Error(w, `{"message":"failed"}`, http.StatusGatewayTimeout)
			return
		}
		fmt.Fprint(w, `{"delay":42}`)
	})
	var tester latencyTester
	var events []LatencyEvent
	result, err := tester.run(c, "https://test.invalid/204", "all", "", func(e LatencyEvent) { events = append(events, e) })
	if err != nil || result.Total != 2 || result.Failed != 1 {
		t.Fatalf("result = %+v, %v", result, err)
	}
	if len(paths) != 2 || paths["/providers/proxies/subscription/shared/healthcheck"] != 1 || paths["/providers/proxies/subscription/sub/healthcheck"] != 1 {
		t.Fatalf("probes = %v; want each unique provider node once, no groups or rejects", paths)
	}
	last := events[len(events)-1]
	if !events[0].Running || len(events[0].Pending) != 2 || last.Running || len(last.Pending) != 0 || last.Completed != 2 || last.Failed != 1 {
		t.Fatalf("unexpected progress: %+v", events)
	}
	delays := map[string]int{}
	for _, e := range events {
		for n, d := range e.Delays {
			delays[n] = d
		}
	}
	if delays["shared"] != 42 || delays["sub"] != -1 {
		t.Fatalf("broadcast delays = %v", delays)
	}
}

func TestLatencyGroupStrategyAndNestedSelections(t *testing.T) {
	proxies := map[string]mihomoapi.Proxy{
		"manual": {Name: "manual", Type: "Selector", All: []string{"nested", "leaf"}, Now: "nested"},
		"nested": {Name: "nested", Type: "Selector", All: []string{"leaf"}, Now: "leaf"},
		"auto":   {Name: "auto", Type: "URLTest", All: []string{"leaf", "dead"}, Now: "leaf"},
		"leaf":   {Name: "leaf", Type: "Shadowsocks"},
		"dead":   {Name: "dead", Type: "Shadowsocks"},
	}
	var requests []string
	var mu sync.Mutex
	c := latencyCore(t, proxies, nil, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.URL.Path+"?"+r.URL.Query().Get("timeout"))
		mu.Unlock()
		if r.URL.Query().Get("url") != "https://global.invalid" {
			t.Errorf("wrong URL: %s", r.URL)
		}
		if strings.HasPrefix(r.URL.Path, "/group/") {
			fmt.Fprint(w, `{"leaf":31}`)
		} else {
			fmt.Fprint(w, `{"delay":31}`)
		}
	})
	var tester latencyTester
	delays := map[string]int{}
	result, err := tester.run(c, "https://global.invalid", "group", "manual", func(e LatencyEvent) {
		for n, d := range e.Delays {
			delays[n] = d
		}
	})
	if err != nil || result.Total != 1 || delays["nested"] != 31 || delays["leaf"] != 31 {
		t.Fatalf("nested test: %+v %v %v", result, delays, err)
	}
	result, err = tester.run(c, "https://global.invalid", "group", "auto", func(LatencyEvent) {})
	if err != nil || result.Total != 2 || result.Failed != 1 {
		t.Fatalf("automatic test: %+v %v", result, err)
	}
	_, err = tester.run(c, "https://global.invalid", "node", "manual", func(LatencyEvent) {})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/proxies/leaf/delay?2000", "/group/auto/delay?5000", "/proxies/leaf/delay?5000"}
	if fmt.Sprint(requests) != fmt.Sprint(want) {
		t.Fatalf("requests = %v, want %v", requests, want)
	}
}

func TestLatencyControllerErrorsClearProgress(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusInternalServerError, http.StatusServiceUnavailable, http.StatusGatewayTimeout} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			c := latencyCore(t, map[string]mihomoapi.Proxy{"node": {Name: "node"}}, nil, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "probe error", status) })
			var tester latencyTester
			var last LatencyEvent
			result, err := tester.run(c, "https://test.invalid", "node", "node", func(e LatencyEvent) { last = e })
			probeFailed := status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
			if (err == nil) != probeFailed {
				t.Fatalf("status %d returned %v", status, err)
			}
			if last.Running || len(last.Pending) != 0 || result.Failed != 1 {
				t.Fatalf("progress not cleared: %+v %+v", last, result)
			}
		})
	}
}

func TestLatencyConcurrencyIsSharedAcrossGroups(t *testing.T) {
	proxies := map[string]mihomoapi.Proxy{}
	for _, group := range []string{"left", "right"} {
		g := mihomoapi.Proxy{Name: group, Type: "Selector"}
		for i := range 8 {
			name := fmt.Sprintf("%s-%d", group, i)
			proxies[name] = mihomoapi.Proxy{Name: name, Type: "Shadowsocks"}
			g.All = append(g.All, name)
		}
		proxies[group] = g
	}
	entered := make(chan struct{}, 16)
	release := make(chan struct{})
	var active, peak atomic.Int32
	c := latencyCore(t, proxies, nil, func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		for before := peak.Load(); n > before; before = peak.Load() {
			if peak.CompareAndSwap(before, n) {
				break
			}
		}
		entered <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
		}
		active.Add(-1)
		fmt.Fprint(w, `{"delay":20}`)
	})
	// Cleanup must release handlers before the test server waits for them.
	t.Cleanup(func() { close(release) })
	var tester latencyTester
	done := make(chan error, 2)
	for _, group := range []string{"left", "right"} {
		go func() {
			_, err := tester.run(c, "https://test.invalid", "group", group, func(LatencyEvent) {})
			done <- err
		}()
	}
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for range 5 {
		select {
		case <-entered:
		case <-deadline.C:
			t.Fatal("five probes did not start")
		}
	}
	// Admit one replacement at a time; no timing sleeps are needed.
	for range 11 {
		release <- struct{}{}
		select {
		case <-entered:
		case <-deadline.C:
			t.Fatal("queued probe did not start")
		}
	}
	for range 5 {
		release <- struct{}{}
	}
	for range 2 {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-deadline.C:
			t.Fatal("tests did not finish")
		}
	}
	if peak.Load() != 5 {
		t.Fatalf("peak concurrent probes = %d, want shared limit of 5", peak.Load())
	}
}
