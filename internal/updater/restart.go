package updater

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// RestartServer launches a new instance of the current executable with the same arguments.
func RestartServer() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return fmt.Errorf("eval symlink: %w", err)
	}

	args := os.Args[1:]
	cmd := exec.Command(exe, args...)
	cmd.Env = append(os.Environ(), "CYRENE_RESTART=1")
	if wd, err := os.Getwd(); err == nil {
		cmd.Dir = wd
	}
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	setDetachedAttrs(cmd)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start new server process: %w", err)
	}

	return nil
}
