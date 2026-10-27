package router

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/localhost-copilot/clashcube/internal/mihomoapi"
	"github.com/localhost-copilot/clashcube/internal/routerupdate"
)

type MaintenanceConfig struct {
	GeoIntervalHours int    `json:"geoIntervalHours"`
	UpdateFeed       string `json:"updateFeed"`
}

// DefaultUpdateFeed is a signed channel pointer. Its manifest pins immutable
// versioned release assets, so advancing the channel cannot change a queued IPK.
const DefaultUpdateFeed = "https://raw.githubusercontent.com/clash-cube/clash-cube/main/openwrt/updates/aarch64_generic.json"

type GeoScheduleState struct {
	Attempt int64 `json:"attempt"`
	Success int64 `json:"success"`
	Failed  bool  `json:"failed"`
}

type Maintenance struct {
	Config MaintenanceConfig `json:"config"`
	Geo    GeoScheduleState  `json:"geo"`
}

func readMaintenance(home string) (MaintenanceConfig, error) {
	c := MaintenanceConfig{UpdateFeed: DefaultUpdateFeed}
	body, err := os.ReadFile(filepath.Join(home, "router-maintenance.json"))
	if os.IsNotExist(err) {
		return c, nil
	}
	if err == nil {
		err = json.Unmarshal(body, &c)
	}
	return c, err
}

func (m *Manager) Maintenance() (Maintenance, error) {
	home := filepath.Dir(m.path)
	c, err := readMaintenance(home)
	var state GeoScheduleState
	if body, e := os.ReadFile(filepath.Join(home, "geo-schedule.json")); e == nil {
		_ = json.Unmarshal(body, &state)
	}
	return Maintenance{Config: c, Geo: state}, err
}

func (m *Manager) SetMaintenance(c MaintenanceConfig) (Maintenance, error) {
	if c.GeoIntervalHours < 0 || c.GeoIntervalHours > 168 {
		return Maintenance{}, errors.New("GEO update interval must be between 0 and 168 hours")
	}
	if c.UpdateFeed != "" {
		if err := routerupdate.ValidateURL(c.UpdateFeed); err != nil {
			return Maintenance{}, err
		}
	}
	if c.GeoIntervalHours > 0 {
		status, err := m.guard.Get()
		if err != nil {
			return Maintenance{}, err
		}
		if status.Access.Mode == "" {
			return Maintenance{}, errors.New("Update the core package before enabling scheduled GEO updates")
		}
	}
	m.opMu.Lock()
	defer m.opMu.Unlock()
	body, err := json.Marshal(c)
	path := filepath.Join(filepath.Dir(m.path), "router-maintenance.json")
	if err == nil {
		err = os.WriteFile(path+".tmp", body, 0600)
	}
	if err == nil {
		err = os.Rename(path+".tmp", path)
	}
	if err != nil {
		return Maintenance{}, err
	}
	return m.Maintenance()
}

func geoDue(config MaintenanceConfig, state GeoScheduleState, now time.Time) bool {
	return config.GeoIntervalHours > 0 && config.GeoIntervalHours <= 168 && now.Unix() >= state.Attempt+int64(config.GeoIntervalHours)*3600
}

// WatchGEO belongs to the core supervisor, so scheduled data updates keep
// working while management is offline. User profiles remain unmodified.
func WatchGEO(ctx context.Context, home string, client func() *mihomoapi.Client) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if err := updateScheduledGEO(ctx, home, client(), now); err != nil {
				log.Printf("scheduled GEO update: %v", err)
			}
		}
	}
}

func updateScheduledGEO(ctx context.Context, home string, c *mihomoapi.Client, now time.Time) error {
	config, err := readMaintenance(home)
	if err != nil || c == nil {
		return err
	}
	path := filepath.Join(home, "geo-schedule.json")
	var state GeoScheduleState
	body, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		if err := json.Unmarshal(body, &state); err != nil {
			return err
		}
	}
	if !geoDue(config, state, now) {
		return nil
	}
	save := func() error {
		body, err := json.Marshal(state)
		if err == nil {
			err = os.WriteFile(path+".tmp", body, 0600)
		}
		if err == nil {
			err = os.Rename(path+".tmp", path)
		}
		return err
	}
	state.Attempt = now.Unix()
	// Record attempts before starting network work, including failed attempts.
	// Respawns and unavailable feeds must not trigger a download every 30 seconds.
	if err := save(); err != nil {
		return err
	}
	updateCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	err = c.UpdateGeo(updateCtx)
	state.Failed = err != nil
	if err == nil {
		state.Success = now.Unix()
	}
	return errors.Join(err, save())
}
