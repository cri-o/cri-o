//go:build seccomp && linux && cgo && test

package seccomp_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	libseccomp "github.com/seccomp/libseccomp-golang"
	"golang.org/x/sys/unix"

	"github.com/cri-o/cri-o/internal/config/seccomp"
)

var errReceiveAgain = errors.New("receive again")

var _ = t.Describe("Notifier", func() {
	t.Describe("handler", func() {
		It("should keep polling notifications until the seccomp fd closes", func() {
			// Given
			msgChan := make(chan seccomp.Notification, 2)
			notifReceive := stubNotifReceive("getpid", "getppid", unix.ENOENT)

			// When
			seccomp.RunHandlerForTest(
				context.Background(),
				"ctr",
				msgChan,
				libseccomp.ScmpFd(-1),
				notifReceive,
				stubNotifIDValid,
				stubNotifRespond,
			)

			// Then
			Expect(msgChan).To(HaveLen(2))
			Expect((<-msgChan).Syscall()).To(Equal("getpid"))
			Expect((<-msgChan).Syscall()).To(Equal("getppid"))
		})

		It("should continue polling after a transient receive error", func() {
			// Given
			msgChan := make(chan seccomp.Notification, 1)
			notifReceive := stubNotifReceive(errReceiveAgain, "getpid", unix.ENOENT)

			// When
			seccomp.RunHandlerForTest(
				context.Background(),
				"ctr",
				msgChan,
				libseccomp.ScmpFd(-1),
				notifReceive,
				stubNotifIDValid,
				stubNotifRespond,
			)

			// Then
			Expect(msgChan).To(HaveLen(1))
			Expect((<-msgChan).Syscall()).To(Equal("getpid"))
		})

		DescribeTable(
			"should stop on a closed seccomp fd error",
			func(terminalErr error) {
				// Given
				msgChan := make(chan seccomp.Notification, 1)
				notifReceive := stubNotifReceive("getpid", terminalErr)

				// When
				seccomp.RunHandlerForTest(
					context.Background(),
					"ctr",
					msgChan,
					libseccomp.ScmpFd(-1),
					notifReceive,
					stubNotifIDValid,
					stubNotifRespond,
				)

				// Then
				Expect(msgChan).To(HaveLen(1))
				Expect((<-msgChan).Syscall()).To(Equal("getpid"))
			},
			Entry("EBADF", unix.EBADF),
			Entry("ECANCELED", unix.ECANCELED),
			Entry("ENOENT", unix.ENOENT),
		)

		It("should skip a notification whose syscall name cannot be decoded", func() {
			// Given
			msgChan := make(chan seccomp.Notification, 1)
			notifReceive := stubNotifReceive(libseccomp.ScmpSyscall(-2), "getpid", unix.ENOENT)

			// When
			seccomp.RunHandlerForTest(
				context.Background(),
				"ctr",
				msgChan,
				libseccomp.ScmpFd(-1),
				notifReceive,
				stubNotifIDValid,
				stubNotifRespond,
			)

			// Then
			Expect(msgChan).To(HaveLen(1))
			Expect((<-msgChan).Syscall()).To(Equal("getpid"))
		})

		It("should continue polling when the TOCTOU check fails", func() {
			// Given
			msgChan := make(chan seccomp.Notification, 1)
			notifReceive := stubNotifReceive("getpid", unix.ENOENT)

			// When
			seccomp.RunHandlerForTest(
				context.Background(),
				"ctr",
				msgChan,
				libseccomp.ScmpFd(-1),
				notifReceive,
				failingNotifIDValid,
				stubNotifRespond,
			)

			// Then
			Expect(msgChan).To(HaveLen(1))
			Expect((<-msgChan).Syscall()).To(Equal("getpid"))
		})

		It("should continue polling when responding fails", func() {
			// Given
			msgChan := make(chan seccomp.Notification, 1)
			notifReceive := stubNotifReceive("getpid", unix.ENOENT)

			// When
			seccomp.RunHandlerForTest(
				context.Background(),
				"ctr",
				msgChan,
				libseccomp.ScmpFd(-1),
				notifReceive,
				stubNotifIDValid,
				failingNotifRespond,
			)

			// Then
			Expect(msgChan).To(HaveLen(1))
			Expect((<-msgChan).Syscall()).To(Equal("getpid"))
		})
	})
})

func stubNotifReceive(events ...any) seccomp.NotifReceiveFunc {
	receiveCalls := 0

	return func(fd libseccomp.ScmpFd) (*libseccomp.ScmpNotifReq, error) {
		Expect(receiveCalls).To(BeNumerically("<", len(events)))

		event := events[receiveCalls]
		receiveCalls++

		switch value := event.(type) {
		case string:
			syscallID, err := libseccomp.GetSyscallFromName(value)
			Expect(err).ToNot(HaveOccurred())

			return &libseccomp.ScmpNotifReq{
				ID: uint64(receiveCalls),
				Data: libseccomp.ScmpNotifData{
					Syscall: syscallID,
				},
			}, nil
		case libseccomp.ScmpSyscall:
			return &libseccomp.ScmpNotifReq{
				ID: uint64(receiveCalls),
				Data: libseccomp.ScmpNotifData{
					Syscall: value,
				},
			}, nil
		case error:
			return nil, value
		default:
			Fail("unsupported notifier receive event")

			return nil, nil
		}
	}
}

func stubNotifIDValid(fd libseccomp.ScmpFd, id uint64) error {
	return nil
}

func stubNotifRespond(fd libseccomp.ScmpFd, resp *libseccomp.ScmpNotifResp) error {
	return nil
}

func failingNotifIDValid(fd libseccomp.ScmpFd, id uint64) error {
	return errors.New("invalid notification ID")
}

func failingNotifRespond(fd libseccomp.ScmpFd, resp *libseccomp.ScmpNotifResp) error {
	return errors.New("unable to respond")
}
