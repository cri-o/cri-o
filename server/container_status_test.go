package server_test

import (
	"context"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/utils"
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
			givenState *oci.ContainerState,
			expectedState pb.ContainerState,
		) {
			// Given
			addContainerAndSandbox()
			testContainer.AddVolume(oci.ContainerVolume{})
			testContainer.SetState(givenState)
			testContainer.SetSpec(&specs.Spec{Version: "1.0.0"})

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
			Expect(response.Info["info"]).To(ContainSubstring(`"ociVersion":"1.0.0"`))
		},
			Entry("Created", &oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateCreated},
			}, pb.ContainerState_CONTAINER_CREATED),
			Entry("Running", &oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			}, pb.ContainerState_CONTAINER_RUNNING),
			Entry("Stopped: ExitCode 0", &oci.ContainerState{
				ExitCode: utils.Int32Ptr(0),
				State:    specs.State{Status: oci.ContainerStateStopped},
			}, pb.ContainerState_CONTAINER_EXITED),
			Entry("Stopped: ExitCode -1", &oci.ContainerState{
				ExitCode: utils.Int32Ptr(-1),
				State:    specs.State{Status: oci.ContainerStateStopped},
			}, pb.ContainerState_CONTAINER_EXITED),
			Entry("Stopped: OOMKilled", &oci.ContainerState{
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
