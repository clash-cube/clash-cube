package appupdate

import (
	"context"
	"errors"
	"net/http"
)

// The signed feed currently contains macOS bundles keyed by CPU architecture.
// Windows checks the feed and links to the release, but must never download or
// install one of those bundles as a Windows update.
var errManualUpdate = errors.New("download the Windows build from the release page")
var ErrCanceled = errors.New("cancelled")

func Bundle() string                  { return "" }
func Stuck(string) string             { return "manual" }
func Writable(string) bool            { return false }
func StageDir(_, cache string) string { return cache }
func Stage(context.Context, *http.Client, *Manifest, string, string, func(int64, int64)) (string, error) {
	return "", errManualUpdate
}
func Install(string, string) error                { return errManualUpdate }
func InstallAsAdmin(string, string, string) error { return errManualUpdate }
func NeedsAdmin(error) bool                       { return false }
func Relaunch(string) error                       { return errManualUpdate }
