package backend

import (
	"context"
	"errors"

	"github.com/localhost-copilot/clashcube/internal/coremgr"
)

var localRunner = &coremgr.AndroidRunner{}
var helperInstalled = func() (bool, bool) { return false, false }

func freshHelper() (bool, bool) { return false, false }

var proxyPointsAt = func(string, int) bool { return false }
var proxyEffective = func(string, int) bool { return false }
var proxyClear = func() error { return nil }
var proxySet = func(string, int, []string) error {
	return errors.New("system proxy is unavailable on Android; use VPN")
}

func (b *Backend) setSystemProxy(bool) error {
	return errors.New("system proxy is unavailable on Android; use VPN")
}

func (b *Backend) pickRunner()                           { b.core.SetRunner(localRunner) }
func (b *Backend) HelperStatus() HelperStatus            { return HelperStatus{} }
func (b *Backend) EnableServiceMode(string) error        { return coremgr.ErrAndroidCore }
func (b *Backend) DisableServiceMode(bool, string) error { return coremgr.ErrAndroidCore }
func (b *Backend) SetTun(bool, string) error             { return coremgr.ErrAndroidCore }
func (b *Backend) setTun(bool, string) error             { return coremgr.ErrAndroidCore }

// Android network ownership and physical-network probes require native APIs.
func bindInterface(uintptr, int) error {
	return errors.New("network binding is not implemented on Android yet")
}
func exclusivePort(uintptr) error            { return nil }
func physicalInterface() string              { return "" }
func serviceOf(string) string                { return "" }
func primaryNetwork() string                 { return "" }
func gateway() string                        { return "" }
func portHolder(int) string                  { return "" }
func pingMS(context.Context, string) float64 { return -1 }

// Desktop update feeds contain desktop bundles, never APKs.
type appUpdater struct{}

func (b *Backend) initUpdater()              {}
func (b *Backend) appUpdateLoop()            {}
func (b *Backend) AppUpdate() AppUpdate      { return AppUpdate{State: "idle", Stuck: "manual"} }
func (b *Backend) CheckAppUpdate() AppUpdate { return b.AppUpdate() }
func (b *Backend) InstallAppUpdate(bool, string) error {
	return errors.New("APK updates are not implemented on Android yet")
}
func (b *Backend) AppBundle() string { return "" }
