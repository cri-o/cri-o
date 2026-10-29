package server_test

import (
	"context"
	"errors"
	"fmt"

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
			context.Background(), enabled, "handler", "exec", `container "id"`, serve,
		)
	}

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
})
