//go:build !windows

package oci

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/creack/pty"
	"github.com/sirupsen/logrus"
	"go.podman.io/storage/pkg/pools"
	"golang.org/x/sys/unix"
	"k8s.io/client-go/tools/remotecommand"

	"github.com/cri-o/cri-o/utils"
)

// ptyStarter wraps pty.Start to implement the ExecStarter interface.
// It stores the pty file descriptor for later use after Start() is called.
type ptyStarter struct {
	cmd *exec.Cmd
	pty *os.File
}

func (p *ptyStarter) Start() error {
	var err error

	p.pty, err = pty.Start(p.cmd)

	return err
}

func (p *ptyStarter) GetPid() int {
	return p.cmd.Process.Pid
}

func (p *ptyStarter) Pty() *os.File {
	return p.pty
}

func Kill(pid int) error {
	err := unix.Kill(pid, unix.SIGKILL)
	if err != nil && !errors.Is(err, unix.ESRCH) {
		return fmt.Errorf("failed to kill process: %w", err)
	}

	return nil
}

func setSize(fd uintptr, size remotecommand.TerminalSize) error {
	winsize := &unix.Winsize{Row: size.Height, Col: size.Width}

	return unix.IoctlSetWinsize(int(fd), unix.TIOCSWINSZ, winsize)
}

func ttyCmd(execCmd *exec.Cmd, stdin io.Reader, stdout io.WriteCloser, resizeChan <-chan remotecommand.TerminalSize, c *Container) error {
	starter := &ptyStarter{cmd: execCmd}

	pid, err := c.StartExecCmd(starter, true)
	if err != nil {
		return err
	}
	defer c.DeleteExecPID(pid)

	p := starter.Pty()
	defer p.Close()
	// make sure to close the stdout stream
	defer stdout.Close()

	utils.HandleResizing(resizeChan, func(size remotecommand.TerminalSize) {
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
