package backend

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/localhost-copilot/clashcube/internal/mihomoapi"
)

func TestAIEgressSharedRequests(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint("failure=", fail), func(t *testing.T) {
			var calls atomic.Int32
			started, release := make(chan struct{}), make(chan struct{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					close(started)
					<-release
				}
				if fail {
					http.Error(w, "unavailable", http.StatusBadGateway)
					return
				}
				fmt.Fprint(w, `{"body":"ip=203.0.113.9\nloc=HK\n"}`)
			}))
			defer srv.Close()
			c := mihomoapi.New(strings.TrimPrefix(srv.URL, "http://"), "")
			var cache aiEgressCache
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var wg sync.WaitGroup
			for range 8 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					body, err := cache.get(ctx, c, "shared-node", proxyTrace)
					if (err != nil) != fail || (!fail && !strings.Contains(body, "203.0.113.9")) {
						t.Errorf("body=%q err=%v", body, err)
					}
				}()
			}
			<-started
			// A cancelled waiter must neither cancel nor duplicate the shared lookup.
			cancelled, stop := context.WithCancel(ctx)
			stop()
			if _, err := cache.get(cancelled, c, "shared-node", proxyTrace); !errors.Is(err, context.Canceled) {
				t.Errorf("cancelled waiter: %v", err)
			}
			close(release)
			wg.Wait()
			_, _ = cache.get(ctx, c, "shared-node", proxyTrace)
			if calls.Load() != 1 {
				t.Fatalf("shared node: %d requests, want 1", calls.Load())
			}
			_, _ = cache.get(ctx, c, "other-node", proxyTrace)
			if calls.Load() != 2 {
				t.Fatalf("distinct node: %d requests, want 2", calls.Load())
			}
			cache.clear()
			_, _ = cache.get(ctx, c, "shared-node", proxyTrace)
			if calls.Load() != 3 {
				t.Fatalf("after reset: %d requests, want 3", calls.Load())
			}
			// A new core must not inherit the previous core's node addresses.
			newCore := mihomoapi.New(strings.TrimPrefix(srv.URL, "http://"), "")
			_, _ = cache.get(ctx, newCore, "shared-node", proxyTrace)
			if calls.Load() != 4 {
				t.Fatalf("new core: %d requests, want 4", calls.Load())
			}
		})
	}
}

func TestAIInspectionUnavailableDoesNotProbe(t *testing.T) {
	var traces atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/clashcube/trace" {
			traces.Add(1)
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	c := mihomoapi.New(strings.TrimPrefix(srv.URL, "http://"), "")
	var cache aiEgressCache
	got := aiCheck(context.Background(), c, aiService{name: "test", hosts: []string{"unreachable.invalid"}}, &cache, proxyTrace)
	if got.Status != "failed" || len(got.Egress) != 0 || traces.Load() != 0 || got.Route.Hosts[0].Error == "" {
		t.Fatalf("unsupported inspection must fail without probes: %+v, traces=%d", got, traces.Load())
	}
}
