//go:build seccomp && linux && cgo && test

// All *_inject.go files are meant to be used by tests only. Purpose of these
// files is to provide a way to inject mocked data into the current setup.

package seccomp

import (
	"context"

	libseccomp "github.com/seccomp/libseccomp-golang"
)

// NotifReceiveFunc is a test hook for libseccomp.NotifReceive.
type NotifReceiveFunc func(libseccomp.ScmpFd) (*libseccomp.ScmpNotifReq, error)

// NotifIDValidFunc is a test hook for libseccomp.NotifIDValid.
type NotifIDValidFunc func(libseccomp.ScmpFd, uint64) error

// NotifRespondFunc is a test hook for libseccomp.NotifRespond.
type NotifRespondFunc func(libseccomp.ScmpFd, *libseccomp.ScmpNotifResp) error

// RunHandlerForTest runs the seccomp notifier handler with injected libseccomp
// calls.
func RunHandlerForTest(
	ctx context.Context,
	containerID string,
	msgChan chan Notification,
	fd libseccomp.ScmpFd,
	notifReceive NotifReceiveFunc,
	notifIDValid NotifIDValidFunc,
	notifRespond NotifRespondFunc,
) {
	notifierHandler{
		notifReceive: notifReceive,
		notifIDValid: notifIDValid,
		notifRespond: notifRespond,
	}.handle(ctx, containerID, msgChan, fd)
}
