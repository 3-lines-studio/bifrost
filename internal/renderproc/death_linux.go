//go:build linux

package renderproc

import (
	"os/exec"
	"syscall"
)

func configureParentDeath(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
}
