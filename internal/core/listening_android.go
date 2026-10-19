package core

import (
	"os"
	"sort"
	"strconv"
	"syscall"
)

// Android denies /proc/net access. Inspect only our own descriptors instead.
func listeningPorts() []int {
	entries, _ := os.ReadDir("/proc/self/fd")
	seen := map[int]bool{}
	out := []int{}
	for _, entry := range entries {
		fd, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		accepting, _ := syscall.GetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_ACCEPTCONN)
		if accepting != 1 {
			continue
		}
		port := 0
		switch a, _ := syscall.Getsockname(fd); v := a.(type) {
		case *syscall.SockaddrInet4:
			port = v.Port
		case *syscall.SockaddrInet6:
			port = v.Port
		}
		if port > 0 && !seen[port] {
			seen[port] = true
			out = append(out, port)
		}
	}
	sort.Ints(out)
	return out
}
