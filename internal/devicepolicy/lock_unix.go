//go:build linux || darwin

package devicepolicy

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

func Lock(home string) (func(), error) {
	if err := os.MkdirAll(home, 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(home, "devices.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return func() { _ = unix.Flock(int(f.Fd()), unix.LOCK_UN); _ = f.Close() }, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) || time.Now().After(deadline) {
			f.Close()
			return nil, errors.New("another device policy change is in progress")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
