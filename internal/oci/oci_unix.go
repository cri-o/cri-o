//go:build !windows

package oci

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/creack/pty"
	"github.com/sirupsen/logrus"
	"go.podman.io/storage/pkg/pools"
	"golang.org/x/sys/unix"
	"k8s.io/cri-streaming/pkg/streaming/remotecommand"

	"github.com/cri-o/cri-o/utils"
)

// ttyTeardownTimeout bounds each teardown step after the TTY command exits,
// so a backpressured output stream cannot gate return indefinitely when the
// streaming idle timeout is disabled. It is a variable so tests can shorten
// it.
var ttyTeardownTimeout = 5 * time.Second

// streamResetter is implemented by SPDY streams, whose Reset fully tears
// down both directions of a stream. WebSocket channels do not implement it,
// so the reset is a no-op there.
type streamResetter interface {
	Reset() error
}

// asyncReset best-effort resets a connection-owned stream without blocking
// teardown on the reset frame write. For SPDY the local remote channels are
// closed before the RST frame is queued, so a reader parked in Stream.Read
// is released even while a graceful output write holds the framer lock.
func asyncReset(s any) {
	if r, ok := s.(streamResetter); ok && r != nil {
		go func() {
			if err := r.Reset(); err != nil {
				logrus.Warnf("Failed to reset stream during teardown: %v", err)
			}
		}()
	}
}

// closeWithTimeout bounds a potentially blocking graceful stream close
// (a FIN write contends for the same framer lock as a stalled data write).
// On timeout the caller should fall back to a full reset and return so the
// connection-level teardown in ServeExec can release the transport.
func closeWithTimeout(c io.Closer, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() {
		done <- c.Close()
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("timed out after %s waiting for stream close", timeout)
	}
}

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

	utils.HandleResizing(resizeChan, func(size remotecommand.TerminalSize) {
		if err := setSize(p.Fd(), size); err != nil {
			logrus.Warnf("Unable to set terminal size: %v", err)
		}
	})

	// Buffered so neither copier blocks on reporting after teardown stops
	// waiting for them.
	stdinDone := make(chan error, 1)
	stdoutDone := make(chan error, 1)

	if stdin != nil {
		go func() {
			_, err := pools.Copy(p, stdin)
			stdinDone <- err
		}()
	} else {
		stdinDone <- nil
	}

	if stdout != nil {
		go func() {
			_, err := pools.Copy(stdout, p)
			stdoutDone <- err
		}()
	} else {
		stdoutDone <- nil
	}

	err = execCmd.Wait()

	// Break the SPDY backpressure cycle before any potentially blocking
	// graceful close: the command has exited, so no further input is needed.
	// Resetting the connection-owned stdin stream releases a copier parked
	// in Stream.Read independently of outbound writes.
	if stdin != nil {
		asyncReset(stdin)
	}

	// Join the copiers, but do not let backpressured output gate return:
	// ServeExec's deferred connection teardown is the actor that releases
	// the transport, and it is only reachable once ttyCmd returns. The PTY
	// is deliberately left open while joining so healthy output still
	// drains instead of being discarded by an early master close.
	timeout := time.After(ttyTeardownTimeout)

	var stdinErr, stdoutErr error

	stdinJoined, stdoutJoined := false, false
	for !stdinJoined || !stdoutJoined {
		select {
		case stdinErr = <-stdinDone:
			stdinJoined = true
		case stdoutErr = <-stdoutDone:
			stdoutJoined = true
		case <-timeout:
			logrus.Warnf("Timed out after %s waiting for TTY copy workers to exit", ttyTeardownTimeout)

			stdinJoined, stdoutJoined = true, true
		}
	}

	// Release copiers parked on the PTY side once draining is done or the
	// join above gave up waiting.
	_ = p.Close()

	// A graceful FIN contends for the same framer lock as a stalled data
	// write, so bound it even when the streaming idle timeout is disabled.
	// On timeout fall back to a full reset so teardown still completes.
	if stdout != nil {
		if closeErr := closeWithTimeout(stdout, ttyTeardownTimeout); closeErr != nil {
			logrus.Warnf("Stalled TTY stdout close, resetting stream: %v", closeErr)
			asyncReset(stdout)
		}
	}

	if stdinErr != nil {
		logrus.Warnf("Stdin copy error: %v", stdinErr)
	}

	if stdoutErr != nil {
		logrus.Warnf("Stdout copy error: %v", stdoutErr)
	}

	return err
}
