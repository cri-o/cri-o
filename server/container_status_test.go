package server_test

import (
	"context"
	"errors"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/storage"
	"github.com/cri-o/cri-o/utils"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// The actual test suite
var _ = t.Describe("ContainerStatus", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
	})

	AfterEach(afterEach)

	t.Describe("ContainerStatus", func() {
		DescribeTable("should succeed", func(
			givenState *oci.ContainerState,
			expectedState types.ContainerState,
			checkpointingEnabled bool,
		) {
			// Given
			if checkpointingEnabled {
				serverConfig.SetCheckpointRestore(true)
			}
			setupSUT()
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
					ContainerId: testContainer.ID(),
				})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
			Expect(len(response.Status.Mounts)).To(BeEquivalentTo(1))
			Expect(response.Status.State).To(Equal(expectedState))
			Expect(response.Info["info"]).To(ContainSubstring(`"ociVersion":"1.0.0"`))
			if checkpointingEnabled {
				Expect(response).To(ContainSubstring(`checkpointedAt`))
			}
		},
			Entry("Created", &oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateCreated},
			}, types.ContainerState_CONTAINER_CREATED, false),
			Entry("Running", &oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			}, types.ContainerState_CONTAINER_RUNNING, false),
			Entry("Running with checkpointing enabled", &oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			}, types.ContainerState_CONTAINER_RUNNING, true),
			Entry("Stopped: ExitCode 0", &oci.ContainerState{
				ExitCode: utils.Int32Ptr(0),
				State:    specs.State{Status: oci.ContainerStateStopped},
			}, types.ContainerState_CONTAINER_EXITED, false),
			Entry("Stopped: ExitCode -1", &oci.ContainerState{
				ExitCode: utils.Int32Ptr(-1),
				State:    specs.State{Status: oci.ContainerStateStopped},
			}, types.ContainerState_CONTAINER_EXITED, false),
			Entry("Stopped: OOMKilled", &oci.ContainerState{
				OOMKilled: true,
				State:     specs.State{Status: oci.ContainerStateStopped},
			}, types.ContainerState_CONTAINER_EXITED, false),
			Entry("Stopped: SeccompKilled", &oci.ContainerState{
				SeccompKilled: true,
				State:         specs.State{Status: oci.ContainerStateStopped},
			}, types.ContainerState_CONTAINER_EXITED, false),
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

		It("should fail with invalid container metadata", func() {
			// Given
			setupSUT()
			addContainerAndSandbox()
			testContainer.AddVolume(oci.ContainerVolume{})
			testContainer.SetStateAndSpoofPid(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			testContainer.SetSpec(&specs.Spec{Version: "1.0.0"})

			gomock.InOrder(
				runtimeServerMock.EXPECT().GetContainerMetadata(gomock.Any()).
					Return(storage.RuntimeContainerMetadata{}, errors.New("not implemented")),
			)
			// When
			response, err := sut.ContainerStatus(context.Background(),
				&types.ContainerStatusRequest{
					Verbose:     true,
					ContainerId: testContainer.ID(),
				})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})
	})
})
