//go:build seccomp && linux && cgo

package seccomp

import (
	"context"
	"errors"
	"testing"
	"time"

	libseccomp "github.com/seccomp/libseccomp-golang"
	"golang.org/x/sys/unix"
)

var errStopHandlerTest = errors.New("stop handler test")

func TestHandlerStopsAfterFirstNotificationInStopMode(t *testing.T) {
	restore := stubNotifierCalls(t, []string{"getpid"}, nil)
	defer restore()

	msgChan := make(chan Notification, 1)

	handler(context.Background(), "ctr", msgChan, libseccomp.ScmpFd(-1), true)

	if got := len(msgChan); got != 1 {
		t.Fatalf("expected 1 notification, got %d", got)
	}

	notification := <-msgChan
	if notification.Syscall() != "getpid" {
		t.Fatalf("expected first syscall to be reported, got %q", notification.Syscall())
	}
}

func TestHandlerKeepsPollingInLogMode(t *testing.T) {
	restore := stubNotifierCalls(t, []string{"getpid", "getppid"}, nil)
	defer restore()

	msgChan := make(chan Notification, 2)

	defer func() {
		if recovered := recover(); recovered != errStopHandlerTest {
			t.Fatalf("expected sentinel panic to stop test handler, got %v", recovered)
		}

		if got := len(msgChan); got != 2 {
			t.Fatalf("expected 2 notifications, got %d", got)
		}

		first := <-msgChan
		second := <-msgChan

		if first.Syscall() != "getpid" {
			t.Fatalf("expected first syscall to be getpid, got %q", first.Syscall())
		}

		if second.Syscall() != "getppid" {
			t.Fatalf("expected second syscall to be getppid, got %q", second.Syscall())
		}
	}()

	handler(context.Background(), "ctr", msgChan, libseccomp.ScmpFd(-1), false)
}

func TestHandlerStopsOnClosedFdError(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "ebadf", err: unix.EBADF},
		{name: "ecanceled", err: unix.ECANCELED},
	} {
		t.Run(tc.name, func(t *testing.T) {
			restore := stubNotifierCalls(t, []string{"getpid"}, tc.err)
			defer restore()

			msgChan := make(chan Notification, 1)
			done := make(chan struct{})

			go func() {
				handler(context.Background(), "ctr", msgChan, libseccomp.ScmpFd(-1), false)
				close(done)
			}()

			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("handler did not exit after closed fd error")
			}

			if got := len(msgChan); got != 1 {
				t.Fatalf("expected 1 notification before closed fd error, got %d", got)
			}

			notification := <-msgChan
			if notification.Syscall() != "getpid" {
				t.Fatalf("expected first syscall to be reported, got %q", notification.Syscall())
			}
		})
	}
}

func stubNotifierCalls(t *testing.T, syscallNames []string, terminalErr error) func() {
	t.Helper()

	receiveCalls := 0

	origReceive := notifReceive
	origIDValid := notifIDValid
	origRespond := notifRespond

	notifReceive = func(fd libseccomp.ScmpFd) (*libseccomp.ScmpNotifReq, error) {
		if receiveCalls >= len(syscallNames) {
			if terminalErr != nil {
				return nil, terminalErr
			}

			panic(errStopHandlerTest)
		}

		syscallName := syscallNames[receiveCalls]
		receiveCalls++

		syscallID, err := libseccomp.GetSyscallFromName(syscallName)
		if err != nil {
			t.Fatalf("resolve syscall %q: %v", syscallName, err)
		}

		return &libseccomp.ScmpNotifReq{
			ID: uint64(receiveCalls),
			Data: libseccomp.ScmpNotifData{
				Syscall: syscallID,
			},
		}, nil
	}

	notifIDValid = func(fd libseccomp.ScmpFd, id uint64) error {
		return nil
	}

	notifRespond = func(fd libseccomp.ScmpFd, resp *libseccomp.ScmpNotifResp) error {
		return nil
	}

	return func() {
		notifReceive = origReceive
		notifIDValid = origIDValid
		notifRespond = origRespond
	}
}
