package updater

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Linux changes /proc/self/exe after a running binary is renamed during update.
var executablePath, executablePathErr = resolveExecutablePath()

func resolveExecutablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

func ExecutablePath() (string, error) {
	return executablePath, executablePathErr
}

// RestartServer launches a new instance of the original executable path.
func RestartServer() error {
	exe, err := ExecutablePath()
	if err != nil {
		return fmt.Errorf("get executable: %w", err)
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
