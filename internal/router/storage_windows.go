package router

import "errors"

func storageBytes(string) (uint64, uint64, error) {
	return 0, 0, errors.New("router diagnostics are unavailable on Windows")
}
