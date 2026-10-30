package server_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"

	conmonClient "github.com/containers/conmon-rs/pkg/client"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cri-o/cri-o/server"
)

// The actual test suite.
var _ = t.Describe("MonitorStreamingURL", func() {
	const monitorURL = "http://127.0.0.1:1234/exec/token"

	var served bool

	BeforeEach(func() {
		served = false
	})

	// enabled reports whether the runtime handler serves the request from
	// the container monitor.
	enabled := func(ok bool, err error) func(string) (bool, error) {
		return func(runtimeHandler string) (bool, error) {
			Expect(runtimeHandler).To(Equal("handler"))

			return ok, err
		}
	}

	// serve returns the URL of the container monitor or fails with err.
	serve := func(err error) func() (string, error) {
		return func() (string, error) {
			served = true

			if err != nil {
				return "", err
			}

			return monitorURL, nil
		}
	}

	monitorStreamingURL := func(enabled func(string) (bool, error), serve func() (string, error)) (string, error) {
		return server.MonitorStreamingURL(
			context.Background(), enabled, "handler", "exec", `container "id"`, false, serve,
		)
	}

	// attachURL is monitorStreamingURL for a request which needs the
	// container monitor.
	attachURL := func(serve func() (string, error)) (string, error) {
		return server.MonitorStreamingURL(
			context.Background(),
			enabled(true, nil),
			"handler",
			"attach",
			`container "id"`,
			true,
			serve,
		)
	}

	// dialError is how the conmon-rs client fails if dialing the socket of
	// its server fails with errno.
	dialError := func(errno syscall.Errno) error {
		return fmt.Errorf("create RPC client: dial long socket: dial unix socket: %w", &net.OpError{
			Op:  "dial",
			Net: "unix",
			Err: os.NewSyscallError("connect", errno),
		})
	}

	// The errors of a container monitor which is unreachable.
	unreachableErrors := []TableEntry{
		Entry("without a socket", dialError(syscall.ENOENT)),
		Entry("with a stale socket", dialError(syscall.ECONNREFUSED)),
		Entry("with a socket of another type", dialError(syscall.EPROTOTYPE)),
		Entry(
			"which disconnected",
			fmt.Errorf("get result: %w", conmonClient.ErrServerDisconnected),
		),
	}

	// containerNotFound is the error of a container monitor which does not
	// know the container.
	containerNotFound := fmt.Errorf("get result: %w", conmonClient.ErrContainerNotFound)

	It("should use the URL of the container monitor", func() {
		// Given
		// When
		url, err := monitorStreamingURL(enabled(true, nil), serve(nil))

		// Then
		Expect(err).ToNot(HaveOccurred())
		Expect(url).To(Equal(monitorURL))
	})

	It("should not ask the container monitor if the handler does not enable it", func() {
		// Given
		// When
		url, err := monitorStreamingURL(enabled(false, nil), serve(nil))

		// Then
		Expect(err).ToNot(HaveOccurred())
		Expect(url).To(BeEmpty())
		Expect(served).To(BeFalse())
	})

	It("should not ask the container monitor for an unknown handler", func() {
		// Given
		// When
		url, err := monitorStreamingURL(enabled(true, errors.New("unknown")), serve(nil))

		// Then
		Expect(err).ToNot(HaveOccurred())
		Expect(url).To(BeEmpty())
		Expect(served).To(BeFalse())
	})

	It("should fall back if the container monitor does not support it", func() {
		// Given
		// When
		url, err := monitorStreamingURL(
			enabled(true, nil), serve(fmt.Errorf("serve exec: %w", errors.ErrUnsupported)),
		)

		// Then
		Expect(err).ToNot(HaveOccurred())
		Expect(url).To(BeEmpty())
		Expect(served).To(BeTrue())
	})

	It("should fail if the container monitor fails", func() {
		// Given
		monitorErr := errors.New("monitor")

		// When
		url, err := monitorStreamingURL(enabled(true, nil), serve(monitorErr))

		// Then
		Expect(err).To(MatchError(monitorErr))
		Expect(err).To(MatchError(ContainSubstring(`could not serve exec for container "id"`)))
		Expect(url).To(BeEmpty())
	})

	It("should fail with ResourceExhausted if too many URLs wait", func() {
		// Given
		// When
		url, err := monitorStreamingURL(
			enabled(true, nil), serve(fmt.Errorf("call RPC: %w", conmonClient.ErrTooManyRequests)),
		)

		// Then
		Expect(status.Code(err)).To(Equal(codes.ResourceExhausted))
		Expect(url).To(BeEmpty())
	})

	DescribeTable("should fall back if the container monitor is unreachable",
		func(monitorErr error) {
			// Given
			// When
			url, err := monitorStreamingURL(enabled(true, nil), serve(monitorErr))

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(url).To(BeEmpty())
			Expect(served).To(BeTrue())
		},
		unreachableErrors,
	)

	It("should fall back if the container monitor does not know the container", func() {
		// Given
		// When
		url, err := monitorStreamingURL(enabled(true, nil), serve(containerNotFound))

		// Then
		Expect(err).ToNot(HaveOccurred())
		Expect(url).To(BeEmpty())
		Expect(served).To(BeTrue())
	})

	It(
		"should fall back if the container monitor does not support a request which needs it",
		func() {
			// Given
			// When
			url, err := attachURL(serve(fmt.Errorf("serve attach: %w", errors.ErrUnsupported)))

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(url).To(BeEmpty())
			Expect(served).To(BeTrue())
		},
	)

	DescribeTable(
		"should fail with Unavailable if the container monitor which is needed is unreachable",
		func(monitorErr error) {
			// Given
			// When
			url, err := attachURL(serve(monitorErr))

			// Then
			Expect(status.Code(err)).To(Equal(codes.Unavailable))
			Expect(
				err,
			).To(MatchError(ContainSubstring(`could not serve attach for container "id"`)))
			Expect(url).To(BeEmpty())
		},
		unreachableErrors,
	)

	It(
		"should fail with NotFound if the container monitor which is needed does not know the container",
		func() {
			// Given
			// When
			url, err := attachURL(serve(containerNotFound))

			// Then
			Expect(status.Code(err)).To(Equal(codes.NotFound))
			Expect(
				err,
			).To(MatchError(ContainSubstring(`could not serve attach for container "id"`)))
			Expect(url).To(BeEmpty())
		},
	)

	It(
		"should fail with ResourceExhausted if too many URLs wait for a request which needs it",
		func() {
			// Given
			// When
			url, err := attachURL(
				serve(fmt.Errorf("call RPC: %w", conmonClient.ErrTooManyRequests)),
			)

			// Then
			Expect(status.Code(err)).To(Equal(codes.ResourceExhausted))
			Expect(url).To(BeEmpty())
		},
	)

	It("should fail if the container monitor which is needed fails", func() {
		// Given
		monitorErr := errors.New("monitor")

		// When
		url, err := attachURL(serve(monitorErr))

		// Then
		Expect(err).To(MatchError(monitorErr))
		Expect(status.Code(err)).To(Equal(codes.Unknown))
		Expect(url).To(BeEmpty())
	})
})
