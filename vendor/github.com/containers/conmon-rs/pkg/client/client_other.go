//go:build !linux

package client

import (
	"errors"
	"net"
	"os"
	"syscall"

	"github.com/sirupsen/logrus"
)

// errPeerPIDUnsupported is returned where the peer credentials of a unix
// socket are not available.
var errPeerPIDUnsupported = errors.New("peer credentials are not supported on this platform")

// errPidfdUnsupported is returned where pidfds are not available.
var errPidfdUnsupported = errors.New("pidfds are not supported on this platform")

// peerPID returns the PID of the process which listens on the other end of
// the connection, which is only supported on Linux.
func peerPID(*net.UnixConn) (int32, error) {
	return 0, errPeerPIDUnsupported
}

// openPidfd returns no pidfd, they are only supported on Linux.
func openPidfd(logrus.FieldLogger, uint32) (pidfd *os.File, gone bool) {
	return nil, false
}

// pidfdSendSignal fails, pidfds are only supported on Linux.
func pidfdSendSignal(*os.File, syscall.Signal) error {
	return errPidfdUnsupported
}

// pidfdExited fails, pidfds are only supported on Linux.
func pidfdExited(*os.File) (bool, error) {
	return false, errPidfdUnsupported
}
