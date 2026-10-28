package coremgr

import (
	"os/exec"
	"syscall"
)

func configureProcess(cmd *exec.Cmd) {
	// The OpenWrt supervisor is the lifetime owner. A SIGKILL must not orphan
	// a core that a respawned supervisor cannot observe or stop.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
}
