package sysproxy

import "errors"

// Transparent routing is owned by the router integration, not desktop proxy settings.
func Set(string, int, []string) error {
	return errors.New("system proxy settings are unavailable on Linux")
}
func Clear() error               { return nil }
func PointsAt(string, int) bool  { return false }
func Effective(string, int) bool { return false }
