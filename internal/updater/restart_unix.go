//go:build !windows

package updater

import (
	"os/exec"
	"syscall"
)

func setDetachedAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}
