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
var _ = t.Describe("ContainerStart", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("ContainerStart", func() {
		It("should succeed", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			addContainerAndSandbox()
			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateCreated},
			})
			gomock.InOrder(
				ociRuntimeMock.EXPECT().StartContainer(gomock.Any()).
					Return(nil),
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any()).
					Return(nil),
			)

			// When
			response, err := sut.StartContainer(context.Background(),
				&pb.StartContainerRequest{
					ContainerId: testContainer.ID(),
				})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
		})

		It("should fail when container start errors", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			addContainerAndSandbox()
			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateCreated},
			})
			gomock.InOrder(
				ociRuntimeMock.EXPECT().StartContainer(gomock.Any()).
					Return(t.TestError),
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any()).
					Return(nil),
			)

			// When
			response, err := sut.StartContainer(context.Background(),
				&pb.StartContainerRequest{
					ContainerId: testContainer.ID(),
				})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

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
	})
})
