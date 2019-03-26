package server_test

import (
	"context"

	"github.com/cri-o/cri-o/oci"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// The actual test suite
var _ = t.Describe("ReopenContainerLog", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("ReopenContainerLog", func() {
		It("should succeed", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			addContainerAndSandbox()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any()).
					Return(nil),
				ociRuntimeMock.EXPECT().ReopenContainerLog(gomock.Any()).
					Return(nil),
			)

			// When
			response, err := sut.ReopenContainerLog(context.Background(),
				&pb.ReopenContainerLogRequest{
					ContainerId: testContainer.ID(),
				})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
		})

		It("should fail when ReopenContainerLog errors", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			addContainerAndSandbox()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any()).
					Return(nil),
				ociRuntimeMock.EXPECT().ReopenContainerLog(gomock.Any()).
					Return(t.TestError),
			)

			// When
			response, err := sut.ReopenContainerLog(context.Background(),
				&pb.ReopenContainerLogRequest{
					ContainerId: testContainer.ID(),
				})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail when container not running", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			addContainerAndSandbox()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any()).
					Return(nil),
			)

			// When
			response, err := sut.ReopenContainerLog(context.Background(),
				&pb.ReopenContainerLogRequest{
					ContainerId: testContainer.ID(),
				})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail when container status update errors", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			addContainerAndSandbox()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any()).
					Return(t.TestError),
			)

			// When
			response, err := sut.ReopenContainerLog(context.Background(),
				&pb.ReopenContainerLogRequest{
					ContainerId: testContainer.ID(),
				})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail on invalid container ID", func() {
			// Given
			// When
			response, err := sut.ReopenContainerLog(context.Background(),
				&pb.ReopenContainerLogRequest{})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})
	})
})
