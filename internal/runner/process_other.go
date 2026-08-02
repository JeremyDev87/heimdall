//go:build !(darwin || linux || freebsd || openbsd || netbsd || dragonfly)

package runner

import (
	"errors"
	"os/exec"
)

func configureProcess(command *exec.Cmd) error {
	return errors.New("hosted process isolation unsupported")
}
func terminateProcess(command *exec.Cmd) error {
	return errors.New("hosted process isolation unsupported")
}
