//go:build windows

package devicepolicy

import "errors"

func Lock(string) (func(), error) { return nil, errors.New("device policies require OpenWrt") }
