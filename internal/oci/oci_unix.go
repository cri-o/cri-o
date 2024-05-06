//go:build !windows
// +build !windows

package oci

import (
	"fmt"
	"io"
	"os/exec"

	"github.com/containers/common/pkg/resize"
	"github.com/containers/storage/pkg/pools"
	"github.com/creack/pty"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

func Kill(pid int) error {
	err := unix.Kill(pid, unix.SIGKILL)
	if err != nil && err != unix.ESRCH {
		return fmt.Errorf("failed to kill process: %w", err)
	}
	return nil
}

func setSize(fd uintptr, size resize.TerminalSize) error {
	winsize := &unix.Winsize{Row: size.Height, Col: size.Width}
	return unix.IoctlSetWinsize(int(fd), unix.TIOCSWINSZ, winsize)
}

func ttyCmd(execCmd *exec.Cmd, stdin io.Reader, stdout io.WriteCloser, resizeChan <-chan resize.TerminalSize, c *Container) error {
	p, err := pty.Start(execCmd)
	if err != nil {
		return err
	}
	defer p.Close()
	// make sure to close the stdout stream
	defer stdout.Close()

	pid := execCmd.Process.Pid
	if err := c.AddExecPID(pid, true); err != nil {
		return err
	}

	defer c.DeleteExecPID(pid)

	resize.HandleResizing(resizeChan, func(size resize.TerminalSize) {
		if err := setSize(p.Fd(), size); err != nil {
			logrus.Warnf("Unable to set terminal size: %v", err)
		}
	})

	var stdinErr, stdoutErr error
	if stdin != nil {
		go func() { _, stdinErr = pools.Copy(p, stdin) }()
	}

	if stdout != nil {
		go func() { _, stdoutErr = pools.Copy(stdout, p) }()
	}

	err = execCmd.Wait()

	if stdinErr != nil {
		logrus.Warnf("Stdin copy error: %v", stdinErr)
	}
	if stdoutErr != nil {
		logrus.Warnf("Stdout copy error: %v", stdoutErr)
	}

	return err
}
