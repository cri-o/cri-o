package server_test

import (
	"context"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/pkg/storage"
	publicOCI "github.com/cri-o/cri-o/pkg/oci"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// The actual test suite
var _ = t.Describe("ContainerStatus", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("ContainerStatus", func() {
		DescribeTable("should succeed", func(
			givenState *publicOCI.ContainerState,
			expectedState pb.ContainerState,
		) {
			// Given
			addContainerAndSandbox()
			testContainer.AddVolume(publicOCI.ContainerVolume{})
			testContainer.SetState(givenState)

			gomock.InOrder(
				imageServerMock.EXPECT().ImageStatus(gomock.Any(),
					gomock.Any()).Return(&storage.ImageResult{}, nil),
			)

			// When
			response, err := sut.ContainerStatus(context.Background(),
				&pb.ContainerStatusRequest{
					Verbose:     true,
					ContainerId: testContainer.ID(),
				})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
			Expect(len(response.Status.Mounts)).To(BeEquivalentTo(1))
			Expect(response.Status.State).To(Equal(expectedState))
		},
			Entry("Created", &publicOCI.ContainerState{
				State: specs.State{Status: oci.ContainerStateCreated},
			}, pb.ContainerState_CONTAINER_CREATED),
			Entry("Running", &publicOCI.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			}, pb.ContainerState_CONTAINER_RUNNING),
			Entry("Stopped: ExitCode 0", &publicOCI.ContainerState{
				ExitCode: 0,
				State:    specs.State{Status: oci.ContainerStateStopped},
			}, pb.ContainerState_CONTAINER_EXITED),
			Entry("Stopped: ExitCode -1", &publicOCI.ContainerState{
				ExitCode: -1,
				State:    specs.State{Status: oci.ContainerStateStopped},
			}, pb.ContainerState_CONTAINER_EXITED),
			Entry("Stopped: OOMKilled", &publicOCI.ContainerState{
				OOMKilled: true,
				State:     specs.State{Status: oci.ContainerStateStopped},
			}, pb.ContainerState_CONTAINER_EXITED),
		)

		It("should fail with invalid container ID", func() {
			// Given
			// When
			response, err := sut.ContainerStatus(context.Background(),
				&pb.ContainerStatusRequest{})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})
	})
})
