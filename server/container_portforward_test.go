package server_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// The actual test suite.
var _ = t.Describe("ContainerPortforward", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("ContainerPortforward", func() {
		It("should succeed", func() {
			// Given
			// When
			response, err := sut.PortForward(context.Background(),
				&types.PortForwardRequest{
					PodSandboxId: testSandbox.ID(),
					Port:         []int32{33300},
				})

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response).NotTo(BeNil())
		})

		It("should fail on missing sandbox ID", func() {
			// Given
			// When
			response, err := sut.PortForward(context.Background(),
				&types.PortForwardRequest{})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(response).To(BeNil())
		})
	})

	t.Describe("StreamServer: PortForward", func() {
		It("should fail when sandbox not found", func() {
			// Given
			// When
			err := testStreamService.PortForward(context.Background(), testSandbox.ID(), 0, nil)

			// Then
			Expect(err).To(HaveOccurred())
		})
	})

	t.Describe("Reverse Port Forwarding", func() {
		Context("isReversePort annotation parsing", func() {
			It("should return true for annotated reverse port", func() {
				// Given
				addContainerAndSandbox()
				testSandbox.SetAnnotations(map[string]string{
					"io.cri-o.reverse-ports": "8080",
				})

				// When
				isReverse := testStreamService.IsReversePort(testSandbox, 8080)

				// Then
				Expect(isReverse).To(BeTrue())
			})

			It("should return true for port in comma-separated list", func() {
				// Given
				addContainerAndSandbox()
				testSandbox.SetAnnotations(map[string]string{
					"io.cri-o.reverse-ports": "8080,9090,3000",
				})

				// When
				isReverse := testStreamService.IsReversePort(testSandbox, 9090)

				// Then
				Expect(isReverse).To(BeTrue())
			})

			It("should return false for non-annotated port", func() {
				// Given
				addContainerAndSandbox()
				testSandbox.SetAnnotations(map[string]string{
					"io.cri-o.reverse-ports": "8080",
				})

				// When
				isReverse := testStreamService.IsReversePort(testSandbox, 9090)

				// Then
				Expect(isReverse).To(BeFalse())
			})

			It("should return false when annotation is missing", func() {
				// Given
				addContainerAndSandbox()

				// When
				isReverse := testStreamService.IsReversePort(testSandbox, 8080)

				// Then
				Expect(isReverse).To(BeFalse())
			})
		})
	})
})
