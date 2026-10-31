package client

import (
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"

	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// peerPID returns the PID of the process which listens on the other end of
// the connection.
func peerPID(conn *net.UnixConn) (int32, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return 0, fmt.Errorf("get raw connection: %w", err)
	}

	var (
		cred    *syscall.Ucred
		credErr error
	)

	if err := raw.Control(func(fd uintptr) {
		cred, credErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil {
		return 0, fmt.Errorf("control raw connection: %w", err)
	}

	if credErr != nil {
		return 0, fmt.Errorf("get peer credentials: %w", credErr)
	}

	return cred.Pid, nil
}

// openPidfd returns a pidfd of the process. Without one, gone tells whether
// the process exited already. Otherwise pidfds are not usable, for example
// because the kernel does not support them, and the process has to be
// handled through its PID.
func openPidfd(logger logrus.FieldLogger, pid uint32) (pidfd *os.File, gone bool) {
	fd, err := unix.PidfdOpen(int(pid), 0)
	if errors.Is(err, unix.ESRCH) {
		return nil, true
	}

	if err != nil {
		logger.WithError(err).Debugf("Unable to open pidfd for PID %d, using the PID", pid)

		return nil, false
	}

	return os.NewFile(uintptr(fd), "pidfd"), false
}

// pidfdSendSignal sends the signal to the process of the pidfd.
func pidfdSendSignal(pidfd *os.File, sig syscall.Signal) error {
	raw, err := pidfd.SyscallConn()
	if err != nil {
		return fmt.Errorf("get raw pidfd: %w", err)
	}

	var sigErr error

	if err := raw.Control(func(fd uintptr) {
		sigErr = unix.PidfdSendSignal(int(fd), sig, nil, 0)
	}); err != nil {
		return fmt.Errorf("control raw pidfd: %w", err)
	}

	return sigErr //nolint:wrapcheck // the caller checks for ESRCH
}

// pidfdExited reports whether the process of the pidfd exited, which
// includes a zombie.
func pidfdExited(pidfd *os.File) (bool, error) {
	raw, err := pidfd.SyscallConn()
	if err != nil {
		return false, fmt.Errorf("get raw pidfd: %w", err)
	}

	var (
		exited  bool
		pollErr error
	)

	if err := raw.Control(func(fd uintptr) {
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}

		var n int

		// A signal, like the preemption of the Go runtime, interrupts the
		// poll, which never gets restarted.
		for {
			n, pollErr = unix.Poll(fds, 0)
			if !errors.Is(pollErr, unix.EINTR) {
				break
			}
		}

		exited = n > 0
	}); err != nil {
		return false, fmt.Errorf("control raw pidfd: %w", err)
	}

	if pollErr != nil {
		return false, fmt.Errorf("poll pidfd: %w", pollErr)
	}

	return exited, nil
}
