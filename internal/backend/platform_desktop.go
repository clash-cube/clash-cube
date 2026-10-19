//go:build !android

package backend

import (
	"github.com/localhost-copilot/clashcube/internal/appupdate"
	"github.com/localhost-copilot/clashcube/internal/helper"
	"github.com/localhost-copilot/clashcube/internal/settings"
	"github.com/localhost-copilot/clashcube/internal/sysproxy"
)

// The system proxy, as variables so tests leave the machine's alone.
var (
	proxySet      = sysproxy.Set
	proxyClear    = sysproxy.Clear
	proxyPointsAt = sysproxy.PointsAt
)

var proxyEffective = sysproxy.Effective
var helperInstalled = helper.Installed

func (b *Backend) initUpdater() {
	if bundle := appupdate.Bundle(); bundle != "" && appupdate.Stuck(bundle) == "" {
		b.upd.bundle = bundle
	}
}
func (b *Backend) setSystemProxy(on bool) error {
	if _, err := settings.Update(func(s *settings.Settings) { s.SystemProxy = on }); err != nil {
		return err
	}
	defer b.emitState()
	if !on {
		b.releaseProxy()
		return nil
	}
	if b.core.Client() == nil {
		return nil // set when the core starts
	}
	return b.applyProxy(true)
}
