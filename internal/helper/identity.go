package helper

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

// Installed says whether the helper is installed and answering, and runs
// the same executable as this app (so an update reinstalls it).
func Installed() (running, current bool) {
	resp, err := Status()
	if err != nil {
		return false, false
	}
	return true, resp.Hash != "" && resp.Hash == selfHash()
}

func selfHash() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	f, err := os.Open(exe)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	_, _ = io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil))
}
