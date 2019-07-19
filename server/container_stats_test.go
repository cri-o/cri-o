package server_test

import (
	"context"

	"github.com/cri-o/cri-o/pkg/oci"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// The actual test suite
var _ = t.Describe("ContainerStats", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("ContainerStats", func() {
		It("should succeed", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			addContainerAndSandbox()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().ContainerStats(gomock.Any()).
					Return(&oci.ContainerStats{}, nil),
			)

			// When
			response, err := sut.ContainerStats(context.Background(),
				&pb.ContainerStatsRequest{
					ContainerId: testContainer.ID(),
				})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
		})

		It("should fail when container stats retrieval errors", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			addContainerAndSandbox()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().ContainerStats(gomock.Any()).
					Return(nil, t.TestError),
			)

			// When
			response, err := sut.ContainerStats(context.Background(),
				&pb.ContainerStatsRequest{
					ContainerId: testContainer.ID(),
				})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail when container stats failed", func() {
			// Given
			addContainerAndSandbox()

			// When
			response, err := sut.ContainerStats(context.Background(),
				&pb.ContainerStatsRequest{
					ContainerId: testContainer.ID(),
				})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail on invalid container", func() {
			// Given
			// When
			response, err := sut.ContainerStats(context.Background(),
				&pb.ContainerStatsRequest{})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})
	})
})
