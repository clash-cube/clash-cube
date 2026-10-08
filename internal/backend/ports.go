package backend

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"syscall"

	"github.com/localhost-copilot/clashcube/internal/modules"
	"github.com/localhost-copilot/clashcube/internal/settings"
)

// checkPorts refuses port modules that ms would open over the profile in
// use on the mixed port, or on a port something else already holds. The
// core only logs a listener it can't open, so it would load and the port
// would silently not be there. Ports before already served are the
// core's own, and not tried.
func checkPorts(before, ms []modules.Module) error {
	s := settings.Load()
	for _, m := range ms {
		if m.Port != nil && m.Enabled && m.Port.Port == s.MixedPort {
			return fmt.Errorf("%s: port %d is the mixed port", m.Name, m.Port.Port)
		}
	}
	held := map[int]bool{}
	for _, p := range served(s.Profile, before) {
		held[p.Port] = true
	}
	for _, p := range served(s.Profile, ms) {
		if !held[p.Port] && !portFree(p.Port) {
			return fmt.Errorf("port %d is in use by another app", p.Port)
		}
	}
	return nil
}

// served is the ports ms open over the profile of that ID.
func served(profile string, ms []modules.Module) []modules.Port {
	var out []modules.Port
	for _, m := range modules.For(profile, ms) {
		if m.Port != nil && m.Port.Target[profile] != "" {
			out = append(out, *m.Port)
		}
	}
	return out
}

// portFree tries the port on the loopback without SO_REUSEADDR, which on
// macOS would let it bind beside another socket's 0.0.0.0. The loopback
// is enough to find a holder and asks the firewall nothing.
func portFree(port int) bool {
	lc := net.ListenConfig{Control: func(_, _ string, c syscall.RawConn) error {
		return c.Control(func(fd uintptr) { _ = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 0) })
	}}
	l, err := lc.Listen(context.Background(), "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = l.Close()
	return true
}

// FreePort is the first port from from up that is free and that neither
// the mixed port nor an enabled port module takes.
func (b *Backend) FreePort(from int) int {
	taken := map[int]bool{settings.Load().MixedPort: true}
	for _, m := range modules.List() {
		if m.Port != nil && m.Enabled {
			taken[m.Port.Port] = true
		}
	}
	for p := max(from, 1024); p <= 65535; p++ {
		if !taken[p] && portFree(p) {
			return p
		}
	}
	return 0
}
