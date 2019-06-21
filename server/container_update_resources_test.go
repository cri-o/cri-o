package server_test

import (
	"context"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// The actual test suite
var _ = t.Describe("UpdateContainerResources", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		mockRuncInLibConfig()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("UpdateContainerResources", func() {
		It("should succeed", func() {
			// Given
			addContainerAndSandbox()
			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})

			// When
			response, err := sut.UpdateContainerResources(context.Background(),
				&pb.UpdateContainerResourcesRequest{
					ContainerId: testContainer.ID()})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
		})

		It("should fail if update container erros", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			addContainerAndSandbox()
			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			gomock.InOrder(
				ociRuntimeMock.EXPECT().UpdateContainer(gomock.Any(),
					gomock.Any()).Return(t.TestError),
			)

			// When
			response, err := sut.UpdateContainerResources(context.Background(),
				&pb.UpdateContainerResourcesRequest{
					ContainerId: testContainer.ID()})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail when container is not in created/running state", func() {
			// Given
			addContainerAndSandbox()

			// When
			response, err := sut.UpdateContainerResources(context.Background(),
				&pb.UpdateContainerResourcesRequest{
					ContainerId: testContainer.ID()})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail with invalid container id", func() {
			// Given
			// When
			response, err := sut.UpdateContainerResources(context.Background(),
				&pb.UpdateContainerResourcesRequest{
					ContainerId: testContainer.ID()})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail with empty container ID", func() {
			// Given
			// When
			response, err := sut.UpdateContainerResources(context.Background(),
				&pb.UpdateContainerResourcesRequest{})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})
	})
})
