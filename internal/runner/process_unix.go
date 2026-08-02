//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package runner

import (
	"os/exec"
	"syscall"
)

func configureProcess(command *exec.Cmd) error {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return nil
}

func terminateProcess(command *exec.Cmd) error {
	if command.Process == nil {
		return nil
	}
	return syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
}
