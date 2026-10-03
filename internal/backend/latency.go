package backend

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/localhost-copilot/mihomobar/internal/mihomoapi"
	"github.com/localhost-copilot/mihomobar/internal/settings"
)

// LatencyEvent describes one test operation. Pending contains display names,
// including group aliases; Delays contains only the newly completed results.
type LatencyEvent struct {
	Key       string         `json:"key"`
	Running   bool           `json:"running"`
	Pending   []string       `json:"pending"`
	Delays    map[string]int `json:"delays"`
	Total     int            `json:"total"`
	Completed int            `json:"completed"`
	Failed    int            `json:"failed"`
}

type LatencyResult struct {
	Total  int `json:"total"`
	Failed int `json:"failed"`
}

// latencyTester is shared by all GUI surfaces. Node tests share five slots;
// concurrent requests for the same operation or node share their result.
type latencyTester struct {
	once       sync.Once
	slots      chan struct{}
	operations singleflight.Group
	nodes      singleflight.Group
}

// TestLatency uses the application's test URL everywhere. kind is node,
// group, provider, or all. Like zashboard's default dashboard mode, selector,
// load-balance and smart groups test individual nodes; automatic groups use
// the core's group endpoint so its selection and health checks are updated.
func (b *Backend) TestLatency(kind, name string, notify func(LatencyEvent)) (LatencyResult, error) {
	c, err := b.Client()
	if err != nil {
		return LatencyResult{}, err
	}
	s := settings.Load()
	return b.latency.run(c, s.TestURL, kind, name, func(e LatencyEvent) {
		// A late result from the previous core/profile must not recolour the
		// current profile's identically named nodes. State events clear progress.
		current, err := b.Client()
		if err == nil && current == c && settings.Load().Profile == s.Profile {
			notify(e)
		}
	})
}

func latencyTestable(p mihomoapi.Proxy) bool {
	switch strings.ToLower(p.Type) {
	case "reject", "rejectdrop", "block":
		return false
	}
	return true
}

// SelectedProxy follows nested groups for the default, non-independent
// latency view. A malformed cycle is stopped rather than hanging the GUI.
func SelectedProxy(all map[string]mihomoapi.Proxy, name string) string {
	seen := map[string]bool{}
	for !seen[name] {
		seen[name] = true
		next := all[name].Now
		if next == "" || next == name {
			break
		}
		if _, ok := all[next]; !ok {
			break
		}
		name = next
	}
	return name
}

func (t *latencyTester) run(c *mihomoapi.Client, url, kind, name string, notify func(LatencyEvent)) (LatencyResult, error) {
	t.once.Do(func() { t.slots = make(chan struct{}, 5) })
	key := kind + "/" + name
	v, err, _ := t.operations.Do(fmt.Sprintf("%p %q %q", c, url, key), func() (any, error) {
		return t.execute(c, url, kind, name, key, notify)
	})
	if v == nil {
		return LatencyResult{}, err
	}
	return v.(LatencyResult), err
}

func (t *latencyTester) execute(c *mihomoapi.Client, url, kind, name, key string, notify func(LatencyEvent)) (LatencyResult, error) {
	ctx := context.Background()
	all, err := c.Proxies(ctx)
	if err != nil {
		return LatencyResult{}, err
	}
	providers, err := c.ProxyProviders(ctx)
	if err != nil {
		return LatencyResult{}, err
	}
	owners := map[string]string{}
	for owner, p := range providers {
		if p.VehicleType == "Compatible" {
			continue
		}
		for _, n := range p.Proxies {
			owners[n.Name] = owner
			if _, ok := all[n.Name]; !ok {
				all[n.Name] = n
			}
		}
	}
	var names []string
	coreGroup := false
	timeout := 2 * time.Second
	switch kind {
	case "node":
		if _, ok := all[name]; !ok {
			return LatencyResult{}, fmt.Errorf("no such proxy: %s", name)
		}
		names = []string{name}
		timeout = 5 * time.Second
	case "group":
		g, ok := all[name]
		if !ok || len(g.All) == 0 {
			return LatencyResult{}, fmt.Errorf("no such proxy group: %s", name)
		}
		names = g.All
		switch strings.ToLower(g.Type) {
		case "selector", "loadbalance", "smart":
		default:
			coreGroup = true
		}
	case "provider":
		p, ok := providers[name]
		if !ok {
			return LatencyResult{}, fmt.Errorf("no such proxy provider: %s", name)
		}
		for _, n := range p.Proxies {
			names = append(names, n.Name)
		}
	case "all":
		for n, p := range all {
			if len(p.All) == 0 {
				names = append(names, n)
			}
		}
		sort.Strings(names)
	default:
		return LatencyResult{}, errors.New("unknown latency test kind")
	}

	// Resolve group aliases before deduplicating, so the same leaf is tested
	// once even when several selectors point to it.
	targets := map[string][]string{}
	for _, n := range names {
		if !latencyTestable(all[n]) {
			continue
		}
		leaf := SelectedProxy(all, n)
		if !latencyTestable(all[leaf]) {
			continue
		}
		target := leaf
		if coreGroup {
			target = n
		}
		targets[target] = append(targets[target], n)
	}
	pending := map[string]bool{}
	for _, aliases := range targets {
		for _, n := range aliases {
			pending[n] = true
		}
	}
	result := LatencyResult{Total: len(targets)}
	completed := 0
	var mu sync.Mutex
	emit := func(running bool, delays map[string]int) {
		waiting := make([]string, 0, len(pending))
		for n := range pending {
			waiting = append(waiting, n)
		}
		sort.Strings(waiting)
		notify(LatencyEvent{Key: key, Running: running, Pending: waiting, Delays: delays, Total: result.Total, Completed: completed, Failed: result.Failed})
	}
	emit(true, nil)
	defer func() { clear(pending); emit(false, nil) }()
	finish := func(target string, d int, testErr error) {
		mu.Lock()
		defer mu.Unlock()
		completed++
		if testErr != nil || d <= 0 {
			result.Failed++
		}
		delays := map[string]int{}
		for _, n := range targets[target] {
			delete(pending, n)
			if testErr == nil {
				delays[n] = d
			}
		}
		if testErr == nil {
			delays[target] = d
		}
		emit(true, delays)
	}
	if coreGroup {
		t.slots <- struct{}{}
		res, err := c.GroupDelay(ctx, name, url, 5*time.Second)
		<-t.slots
		if err != nil {
			return result, err
		}
		for target := range targets {
			d := res[target]
			if d <= 0 {
				d = -1
			}
			finish(target, d, nil)
		}
		return result, nil
	}
	var wg sync.WaitGroup
	var firstErr error
	for target := range targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d, err := t.node(c, target, owners[target], url, timeout)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
			finish(target, d, err)
		}()
	}
	wg.Wait()
	return result, firstErr
}

func (t *latencyTester) node(c *mihomoapi.Client, name, owner, url string, timeout time.Duration) (int, error) {
	key := fmt.Sprintf("%p %q %q %q %d", c, name, owner, url, timeout)
	v, err, _ := t.nodes.Do(key, func() (any, error) {
		t.slots <- struct{}{}
		defer func() { <-t.slots }()
		var d int
		var err error
		if owner != "" {
			d, err = c.ProviderProxyDelay(context.Background(), owner, name, url, timeout)
		} else {
			d, err = c.Delay(context.Background(), name, url, timeout)
		}
		var ae *mihomoapi.APIError
		// mihomo reports a failed probe as 503 or 504. Authentication, missing nodes
		// and controller errors must remain errors rather than fake timeouts.
		if errors.As(err, &ae) && (ae.Status == http.StatusGatewayTimeout || ae.Status == http.StatusServiceUnavailable) {
			return -1, nil
		}
		if err == nil && d <= 0 {
			d = -1
		}
		return d, err
	})
	return v.(int), err
}
