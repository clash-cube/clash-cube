package router

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/localhost-copilot/clashcube/internal/mihomoapi"
)

func TestMaintenanceDefaultsToOfficialFeedAndPreservesExplicitOptOut(t *testing.T) {
	home := t.TempDir()
	c, err := readMaintenance(home)
	if err != nil || c.UpdateFeed != DefaultUpdateFeed || c.GeoIntervalHours != 0 {
		t.Fatal("fresh router did not select the official feed", c, err)
	}
	m := &Manager{path: filepath.Join(home, "router.json"), guard: &policyGuard{}}
	if _, err := m.SetMaintenance(MaintenanceConfig{}); err != nil {
		t.Fatal(err)
	}
	c, err = readMaintenance(home)
	if err != nil || c.UpdateFeed != "" {
		t.Fatal("explicitly disabled feed was replaced", c, err)
	}
}

func TestScheduledGEORetainsAttemptAcrossRestartAndRecovers(t *testing.T) {
	home := t.TempDir()
	m := &Manager{path: filepath.Join(home, "router.json"), guard: &policyGuard{}}
	if _, err := m.SetMaintenance(MaintenanceConfig{GeoIntervalHours: 1}); err != nil {
		t.Fatal(err)
	}
	calls, fail := 0, true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/upgrade/geo" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer private" {
			t.Error("incorrect scheduled core request")
		}
		calls++
		if fail {
			http.Error(w, "fixture unavailable", 500)
		} else {
			w.WriteHeader(204)
		}
	}))
	defer server.Close()
	client := mihomoapi.New(strings.TrimPrefix(server.URL, "http://"), "private")
	now := time.Unix(100000, 0)
	if err := updateScheduledGEO(context.Background(), home, client, now); err == nil {
		t.Fatal("expected failed update")
	}
	if err := updateScheduledGEO(context.Background(), home, client, now.Add(3599*time.Second)); err != nil || calls != 1 {
		t.Fatal("respawn repeated a failed update before its interval", err)
	}
	fail = false
	if err := updateScheduledGEO(context.Background(), home, client, now.Add(time.Hour)); err != nil || calls != 2 {
		t.Fatal("scheduled recovery failed", err)
	}
	stored, err := m.Maintenance()
	if err != nil || stored.Geo.Failed || stored.Geo.Success != now.Add(time.Hour).Unix() {
		t.Fatal("successful update state was not persisted", err)
	}
	if _, err := m.SetMaintenance(MaintenanceConfig{}); err != nil {
		t.Fatal(err)
	}
	if err := updateScheduledGEO(context.Background(), home, client, now.Add(24*time.Hour)); err != nil || calls != 2 {
		t.Fatal("disabled scheduler sent a request", err)
	}
	var state GeoScheduleState
	body, _ := os.ReadFile(filepath.Join(home, "geo-schedule.json"))
	if json.Unmarshal(body, &state) != nil || state.Success == 0 {
		t.Fatal("management changes erased scheduled history")
	}
}
