package core

import (
	"sort"
	"syscall"
)

// listeningPorts is the TCP ports this process listens on, read from its
// own descriptors: a stream socket with a port and no peer. darwin has no
// SO_ACCEPTCONN, and listing /dev/fd stops at its own descriptor, so every
// descriptor up to the limit is tried.
func listeningPorts() []int {
	var lim syscall.Rlimit
	n := 1 << 16
	if syscall.Getrlimit(syscall.RLIMIT_NOFILE, &lim) == nil && lim.Cur < uint64(n) {
		n = int(lim.Cur)
	}
	seen := map[int]bool{}
	out := []int{}
	for fd := 0; fd < n; fd++ {
		if typ, err := syscall.GetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_TYPE); err != nil || typ != syscall.SOCK_STREAM {
			continue
		}
		if _, err := syscall.Getpeername(fd); err != syscall.ENOTCONN {
			continue
		}
		var port int
		switch sa, _ := syscall.Getsockname(fd); a := sa.(type) {
		case *syscall.SockaddrInet4:
			port = a.Port
		case *syscall.SockaddrInet6:
			port = a.Port
		}
		if port > 0 && !seen[port] {
			seen[port] = true
			out = append(out, port)
		}
	}
	sort.Ints(out)
	return out
}
