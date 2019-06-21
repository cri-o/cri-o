package server_test

import (
	"context"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// The actual test suite
var _ = t.Describe("ContainerStatsList", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("ContainerStatsList", func() {
		It("should succeed", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			addContainerAndSandbox()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().ContainerStats(gomock.Any()).
					Return(&oci.ContainerStats{}, nil),
			)

			// When
			response, err := sut.ListContainerStats(context.Background(),
				&pb.ListContainerStatsRequest{
					Filter: &pb.ContainerStatsFilter{},
				})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
		})

		It("should succeed if container stat retrieval errors", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			addContainerAndSandbox()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().ContainerStats(gomock.Any()).
					Return(nil, t.TestError),
			)

			// When
			response, err := sut.ListContainerStats(context.Background(),
				&pb.ListContainerStatsRequest{
					Filter: &pb.ContainerStatsFilter{},
				})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
		})
	})
})
