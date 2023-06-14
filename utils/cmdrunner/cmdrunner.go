package cmdrunner

import (
	"os/exec"
)

// Use a singleton instance because there are many modules that may want access
// and having it all go through a shared object like the config or server would
// add a lot of complexity.
var commandRunner CommandRunner

// CommandRunner is an interface for executing commands.
// It gives the option to change the way commands are run server-wide.
type CommandRunner interface {
	Command(string, ...string) *exec.Cmd
	CombinedOutput(string, ...string) ([]byte, error)
}

// prependableCommandRunner is an implementation of CommandRunner.
// It gives the option for all commands that are run to be prepended by another command
// and arguments.
type prependableCommandRunner struct {
	prependCmd  string
	prependArgs []string
}

// PrependCommandsWith updates the commandRunner singleton to have the configured prepended args and command.
func PrependCommandsWith(prependCmd string, prependArgs ...string) {
	commandRunner = &prependableCommandRunner{
		prependCmd:  prependCmd,
		prependArgs: prependArgs,
	}
}

// CombinedOutput calls CombinedOutput on the defined commandRunner,
// or the default implementation in the exec package if there's no commandRunner defined.
func CombinedOutput(command string, args ...string) ([]byte, error) {
	if commandRunner == nil {
		return exec.Command(command, args...).CombinedOutput()
	}
	return commandRunner.CombinedOutput(command, args...)
}

// CombinedOutput returns the combined output of the command, given the prepended cmd/args that were defined.
func (c *prependableCommandRunner) CombinedOutput(command string, args ...string) ([]byte, error) {
	return c.Command(command, args...).CombinedOutput()
}

// Command calls Command on the defined commandRunner,
// or the default implementation in the exec package if there's no commandRunner defined.
func Command(cmd string, args ...string) *exec.Cmd {
	if commandRunner == nil {
		return exec.Command(cmd, args...)
	}
	return commandRunner.Command(cmd, args...)
}

// Command creates an exec.Cmd object. If prependCmd is defined, the command will be prependCmd
// and the args will be prependArgs + cmd + args.
// Otherwise, cmd and args will be as inputted.
func (c *prependableCommandRunner) Command(cmd string, args ...string) *exec.Cmd {
	realCmd := cmd
	realArgs := args
	if c.prependCmd != "" {
		realCmd = c.prependCmd
		realArgs = c.prependArgs
		realArgs = append(realArgs, cmd)
		realArgs = append(realArgs, args...)
	}
	return exec.Command(realCmd, realArgs...)
}

// GetPrependedCmd returns the prepended command if one is configured, else the empty string
func GetPrependedCmd() string {
	if c, ok := commandRunner.(*prependableCommandRunner); ok {
		return c.prependCmd
	}
	return ""
}

// ResetPrependedCmd resets the singleton for more reliable unit testing
func ResetPrependedCmd() {
	commandRunner = nil
}
