package server_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/runtime-spec/specs-go"
	"k8s.io/client-go/tools/remotecommand"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/oci"
)

// mockExecStarter is a mock implementation of oci.ExecStarter for testing.
type mockExecStarter struct {
	pid int
}

func (m *mockExecStarter) Start() error {
	return nil
}

func (m *mockExecStarter) GetPid() int {
	return m.pid
}

// The actual test suite.
var _ = t.Describe("ContainerExec", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("ContainerExec", func() {
		It("should succeed", func() {
			// Given
			addContainerAndSandbox()

			// When
			response, err := sut.Exec(context.Background(),
				&types.ExecRequest{
					ContainerId: testContainer.ID(),
					Stdout:      true,
				})

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response).NotTo(BeNil())
		})

		It("should fail on invalid request", func() {
			// Given
			// When
			response, err := sut.Exec(context.Background(),
				&types.ExecRequest{})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(response).To(BeNil())
		})
	})

	t.Describe("StreamServer: Exec", func() {
		It("should fail when container not found", func() {
			// Given
			// When
			err := testStreamService.Exec(context.Background(), testContainer.ID(), []string{},
				nil, nil, nil, false, make(chan remotecommand.TerminalSize))

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should succeed when container is running", func() {
			// Given
			addContainerAndSandbox()
			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			testContainer.SetStateAndSpoofPid(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})

			// When
			err := testStreamService.Exec(context.Background(), testContainer.ID(), []string{"/bin/sh"},
				nil, nil, nil, false, make(chan remotecommand.TerminalSize))

			// Then - Should succeed because container is running
			Expect(err).To(HaveOccurred()) // Will fail trying to actually exec but won't fail the Living() check
			Expect(err.Error()).NotTo(ContainSubstring("container is not created or running"))
		})

		It("should allow exec to be attempted during graceful termination", func() {
			// Given
			addContainerAndSandbox()
			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			// Container is stopping but kill loop hasn't started
			testContainer.SetAsStopping()

			// When - Try to start an exec during graceful termination
			mockStarter := &mockExecStarter{pid: 12345}
			pid, err := testContainer.StartExecCmd(mockStarter, true)

			// Then - Should succeed because stopKillLoopBegun is still false
			Expect(err).ToNot(HaveOccurred())
			Expect(pid).To(Equal(12345))
		})

		It("should fail when container process is not alive", func() {
			// Given
			addContainerAndSandbox()
			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateStopped},
			})

			// When
			err := testStreamService.Exec(context.Background(), testContainer.ID(), []string{"/bin/sh"},
				nil, nil, nil, false, make(chan remotecommand.TerminalSize))

			// Then - Should fail the Living() check
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("container is not created or running"))
		})
	})
})
