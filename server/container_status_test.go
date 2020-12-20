package server_test

import (
	"context"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/storage"
	"github.com/cri-o/cri-o/server/cri/types"
	"github.com/cri-o/cri-o/utils"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/runtime-spec/specs-go"
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
			givenState *oci.ContainerState,
			expectedState types.ContainerState,
		) {
			// Given
			addContainerAndSandbox()
			testContainer.AddVolume(oci.ContainerVolume{})
			testContainer.SetStateAndSpoofPid(givenState)
			testContainer.SetSpec(&specs.Spec{Version: "1.0.0"})

			gomock.InOrder(
				runtimeServerMock.EXPECT().GetContainerMetadata(gomock.Any()).
					Return(storage.RuntimeContainerMetadata{}, nil),
			)

			// When
			response, err := sut.ContainerStatus(context.Background(),
				&types.ContainerStatusRequest{
					Verbose:     true,
					ContainerID: testContainer.ID(),
				})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
			Expect(len(response.Status.Mounts)).To(BeEquivalentTo(1))
			Expect(response.Status.State).To(Equal(expectedState))
			Expect(response.Info["info"]).To(ContainSubstring(`"ociVersion":"1.0.0"`))
		},
			Entry("Created", &oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateCreated},
			}, types.ContainerStateContainerCreated),
			Entry("Running", &oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			}, types.ContainerStateContainerRunning),
			Entry("Stopped: ExitCode 0", &oci.ContainerState{
				ExitCode: utils.Int32Ptr(0),
				State:    specs.State{Status: oci.ContainerStateStopped},
			}, types.ContainerStateContainerExited),
			Entry("Stopped: ExitCode -1", &oci.ContainerState{
				ExitCode: utils.Int32Ptr(-1),
				State:    specs.State{Status: oci.ContainerStateStopped},
			}, types.ContainerStateContainerExited),
			Entry("Stopped: OOMKilled", &oci.ContainerState{
				OOMKilled: true,
				State:     specs.State{Status: oci.ContainerStateStopped},
			}, types.ContainerStateContainerExited),
		)

		It("should fail with invalid container ID", func() {
			// Given
			// When
			response, err := sut.ContainerStatus(context.Background(),
				&types.ContainerStatusRequest{})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})
	})
})
