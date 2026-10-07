package backend

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAIIPCache(t *testing.T) {
	var cache aiIPCache
	now := time.Now()
	calls := 0
	lookup := func(context.Context, string) (*AIIPDetails, error) {
		calls++
		return &AIIPDetails{City: fmt.Sprint(calls)}, nil
	}
	get := func(ip string, force bool, at time.Time) *AIIPDetails {
		t.Helper()
		v, err := cache.get(context.Background(), ip, force, at, lookup)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	first := get("1.1.1.1", false, now)
	if got := get("1.1.1.1", false, now.Add(9*time.Minute)); got != first || calls != 1 {
		t.Fatal("cache miss before expiry")
	}
	get("8.8.8.8", false, now)
	if calls != 2 {
		t.Fatal("changed IP did not query")
	}
	get("1.1.1.1", true, now.Add(time.Minute))
	if calls != 3 {
		t.Fatal("forced refresh reused cache")
	}
	get("1.1.1.1", false, now.Add(11*time.Minute))
	if calls != 4 {
		t.Fatal("expired cache reused")
	}

	failures := 0
	fail := func(context.Context, string) (*AIIPDetails, error) { failures++; return nil, errors.New("unavailable") }
	for _, elapsed := range []time.Duration{0, 29 * time.Second, 30 * time.Second} {
		_, _ = cache.get(context.Background(), "9.9.9.9", false, now.Add(elapsed), fail)
	}
	if failures != 2 {
		t.Fatalf("failure retry count: %d", failures)
	}
}

func TestAIIPCachePendingAndCancellation(t *testing.T) {
	var cache aiIPCache
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	lookup := func(context.Context, string) (*AIIPDetails, error) {
		close(started)
		<-release
		return &AIIPDetails{City: "cached"}, nil
	}
	now := time.Now()
	go func() { defer close(finished); _, _ = cache.get(context.Background(), "1.1.1.1", false, now, lookup) }()
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := cache.get(ctx, "1.1.1.1", true, now, lookup); !errors.Is(err, context.Canceled) {
		t.Errorf("waiting lookup did not cancel: %v", err)
	}
	close(release)
	<-finished
	got, err := cache.get(context.Background(), "1.1.1.1", false, now, lookup)
	if err != nil || got.City != "cached" {
		t.Fatalf("cancelled waiter affected owner: %+v, %v", got, err)
	}
}

func TestAIIPAttributes(t *testing.T) {
	for _, tc := range []struct {
		name, body, kind, network string
		status                    int
		fail                      bool
	}{
		{"ISP is not proof of residential", `{"ip":"72.234.229.123","city":"Aiea","subdivision":"Hawaii","company":{"name":"Hawaiian Telcom","type":"isp"},"asn":{"asn":"AS36149","route":"72.234.229.0/24"}}`, "ISP", "72.234.229.0/24", 200, false},
		{"hosting", `{"ip":"72.234.229.123","company":{"type":"hosting"}}`, "Datacenter", "", 200, false},
		{"hosting overrides ISP", `{"ip":"72.234.229.123","company":{"type":"isp"},"privacy":{"is_hosting":true}}`, "Datacenter", "", 200, false},
		{"ASN fallback", `{"ip":"72.234.229.123","city":"Aiea","subdivision":"Hawaii","company":null,"asn":{"asn":"AS36149","name":"Hawaiian Telcom","type":"isp","route":"72.234.229.0/24"}}`, "ISP", "72.234.229.0/24", 200, false},
		{"unknown provider category", `{"ip":"72.234.229.123","company":{"type":"other"}}`, "Unknown", "", 200, false},
		{"invalid network ignored", `{"ip":"72.234.229.123","asn":{"route":"1.1.1.0/24"}}`, "Unknown", "", 200, false},
		{"missing attributes stay unknown", `{"ip":"72.234.229.123"}`, "Unknown", "", 200, false},
		{"unrelated response", `{"ip":"1.1.1.1","asn":{"route":"1.1.1.0/24"}}`, "", "", 200, true},
		{"another address in same network", `{"ip":"72.234.229.132","asn":{"route":"72.234.229.0/24"}}`, "", "", 200, true},
		{"empty response", `{}`, "", "", 200, true},
		{"invalid JSON", `<html>unavailable</html>`, "", "", 200, true},
		{"rate limited", `{}`, "", "", 429, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/72.234.229.123" {
					t.Errorf("wrong lookup: %s", r.URL.Path)
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()
			got, err := fetchAIIP(context.Background(), srv.Client(), srv.URL+"/", "72.234.229.123")
			if tc.fail {
				if err == nil || got != nil {
					t.Fatalf("accepted invalid response: %+v, %v", got, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Kind != tc.kind || got.Network != tc.network {
				t.Fatalf("unexpected attributes: %+v", got)
			}
			if tc.network != "" && (got.City != "Aiea" || got.Region != "Hawaii" || got.Operator != "Hawaiian Telcom" || got.ASN != 36149) {
				t.Fatalf("missing attributes: %+v", got)
			}
		})
	}
}

func TestAIIPRejectsNonpublicAddresses(t *testing.T) {
	for _, ip := range []string{"", "not-an-ip", "127.0.0.1", "192.168.1.1", "::1", "fe80::1"} {
		if _, err := fetchAIIP(context.Background(), nil, "", ip); err == nil {
			t.Errorf("accepted %q", ip)
		}
	}
}
