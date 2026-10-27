package core

import (
	"os"
	"slices"
	"strconv"
	"syscall"
)

func listeningPorts() []int {
	fds, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return nil
	}
	seen := map[int]bool{}
	var ports []int
	for _, entry := range fds {
		fd, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		listening, err := syscall.GetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_ACCEPTCONN)
		if err != nil || listening != 1 {
			continue
		}
		var port int
		switch a, _ := syscall.Getsockname(fd); a := a.(type) {
		case *syscall.SockaddrInet4:
			port = a.Port
		case *syscall.SockaddrInet6:
			port = a.Port
		}
		if port > 0 && !seen[port] {
			seen[port] = true
			ports = append(ports, port)
		}
	}
	slices.Sort(ports)
	return ports
}
