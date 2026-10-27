//go:build linux || darwin

package router

import "golang.org/x/sys/unix"

func storageBytes(path string) (total, free uint64, err error) {
	var s unix.Statfs_t
	if err = unix.Statfs(path, &s); err != nil {
		return 0, 0, err
	}
	return uint64(s.Blocks) * uint64(s.Bsize), uint64(s.Bavail) * uint64(s.Bsize), nil
}
