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
})
