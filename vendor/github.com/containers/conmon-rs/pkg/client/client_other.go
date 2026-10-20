//go:build !linux

package client

import (
	"errors"
	"net"
)

// errPeerPIDUnsupported is returned where the peer credentials of a unix
// socket are not available.
var errPeerPIDUnsupported = errors.New("peer credentials are not supported on this platform")

// peerPID returns the PID of the process which listens on the other end of
// the connection, which is only supported on Linux.
func peerPID(*net.UnixConn) (int32, error) {
	return 0, errPeerPIDUnsupported
}
