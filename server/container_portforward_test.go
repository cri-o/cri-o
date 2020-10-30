package server_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// The actual test suite
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
				&pb.PortForwardRequest{
					PodSandboxId: testSandbox.ID(),
					Port:         []int32{33300},
				})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
		})

		It("should fail on missing sandbox ID", func() {
			// Given
			// When
			response, err := sut.PortForward(context.Background(),
				&pb.PortForwardRequest{})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})
	})

	t.Describe("StreamServer: PortForward", func() {
		It("shoud fail when sandbox not found", func() {
			// Given
			// When
			err := testStreamService.PortForward(testSandbox.ID(), 0, nil)

			// Then
			Expect(err).NotTo(BeNil())
		})
	})
})
