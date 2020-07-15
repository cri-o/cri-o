package server_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// The actual test suite
var _ = t.Describe("ContainerStart", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("ContainerStart", func() {
		It("should fail with container not in created state", func() {
			// Given
			addContainerAndSandbox()

			// When
			response, err := sut.StartContainer(context.Background(),
				&pb.StartContainerRequest{
					ContainerId: testContainer.ID(),
				})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail with invalid container ID", func() {
			// Given
			// When
			response, err := sut.StartContainer(context.Background(),
				&pb.StartContainerRequest{})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail with invalid container state", func() {
			// Given
			addContainerAndSandbox()

			// When
			response, err := sut.StartContainer(context.Background(),
				&pb.StartContainerRequest{
					ContainerId: testContainer.ID(),
				},
			)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})
	})
})
