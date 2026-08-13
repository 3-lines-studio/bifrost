//go:build !linux && !windows

package renderproc

import "os/exec"

func configureParentDeath(*exec.Cmd) {}
