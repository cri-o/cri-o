package cmdrunner

import (
	"os/exec"
)

type CommandRunner interface {
	CombinedOutput(string, ...string) ([]byte, error)
}

type RealCommandRunner struct{}

func (c *RealCommandRunner) CombinedOutput(command string, args ...string) ([]byte, error) {
	return exec.Command(command, args...).CombinedOutput()
}
