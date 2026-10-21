package client

import (
	"fmt"
	"net"
	"syscall"
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
