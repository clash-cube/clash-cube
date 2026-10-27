package appupdate

import (
	"context"
	"errors"
	"net/http"
)

// Router packages are upgraded by the package manager, never the desktop updater.
var errManualUpdate = errors.New("update the router package with its package manager")
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
