//go:build seccomp && linux && cgo && test

package seccomp

import (
	"testing"

	"golang.org/x/sys/unix"
)

// The descriptor returned by handleNewMessage must be close-on-exec, otherwise
// the container monitor inherits it and keeps the notify listener attached.
func TestHandleNewMessageSetsCloseOnExec(t *testing.T) {
	socks, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}

	defer unix.Close(socks[0])
	defer unix.Close(socks[1])

	// Hand socks[0] over as if it were the seccomp fd (specs.SeccompFdName).
	state := []byte(`{"fds":["seccompFd"]}`)

	err = unix.Sendmsg(socks[0], state, unix.UnixRights(socks[0]), nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	fd, err := handleNewMessage(socks[1])
	if err != nil {
		t.Fatal(err)
	}

	defer unix.Close(int(fd))

	flags, err := unix.FcntlInt(fd, unix.F_GETFD, 0)
	if err != nil {
		t.Fatal(err)
	}

	if flags&unix.FD_CLOEXEC == 0 {
		t.Error("received descriptor is not close-on-exec")
	}
}
