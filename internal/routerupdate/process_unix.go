//go:build linux || darwin

package routerupdate

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

func workerAlive(pid int) bool {
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	// Avoid mistaking a recycled pid for our worker after a crash.
	body, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
	return err == nil && strings.Contains(string(body), "\x00update-core\x00")
}
