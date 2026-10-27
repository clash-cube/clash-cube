package router

import (
	"errors"
	"path/filepath"

	"github.com/localhost-copilot/clashcube/internal/appdir"
	"github.com/localhost-copilot/clashcube/internal/routerupdate"
)

func (m *Manager) Updates() (routerupdate.State, error) {
	return routerupdate.New(filepath.Join(appdir.CacheHome(), "updates")).State()
}

func (m *Manager) CheckUpdates() (routerupdate.State, error) {
	c, err := readMaintenance(filepath.Dir(m.path))
	if err != nil {
		return routerupdate.State{}, err
	}
	if c.UpdateFeed == "" {
		return routerupdate.State{}, errors.New("configure a signed core update feed first")
	}
	return routerupdate.New(filepath.Join(appdir.CacheHome(), "updates")).Check(c.UpdateFeed)
}

func (m *Manager) UpdateCore() (routerupdate.State, error) {
	c, err := readMaintenance(filepath.Dir(m.path))
	if err != nil {
		return routerupdate.State{}, err
	}
	return routerupdate.New(filepath.Join(appdir.CacheHome(), "updates")).Apply(c.UpdateFeed)
}
