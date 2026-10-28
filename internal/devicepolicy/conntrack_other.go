//go:build !linux

package devicepolicy

import "errors"

func Evict(string, string) error { return errors.New("device protection requires Linux") }
