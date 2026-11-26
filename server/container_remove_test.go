package server_test

import (
	"context"
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"go.uber.org/mock/gomock"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/oci"
)

// The actual test suite.
var _ = t.Describe("ContainerRemove", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		mockRuntimeInLibConfig()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("ContainerRemove", func() {
		It("should succeed", func() {
			// Given
			addContainerAndSandbox()
			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateStopped},
			})
			gomock.InOrder(
				runtimeServerMock.EXPECT().DeleteContainer(gomock.Any(), gomock.Any()).
					Return(nil),
			)
			// This allows us to skip stopContainer() which fails because we don't
			// spoof the `runtime state` call in `UpdateContainerStatus`
			testSandbox.SetStopped(context.Background(), false)

			// When
			_, err := sut.RemoveContainer(context.Background(),
				&types.RemoveContainerRequest{
					ContainerId: testContainer.ID(),
				})

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should not keep a runtime implementation for the removed container", func() {
			// Given
			addContainerAndSandbox()
			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateStopped},
			})
			runtimeServerMock.EXPECT().DeleteContainer(gomock.Any(), gomock.Any()).
				Return(nil)
			testSandbox.SetStopped(context.Background(), false)

			// When
			_, err := sut.RemoveContainer(context.Background(),
				&types.RemoveContainerRequest{
					ContainerId: testContainer.ID(),
				})

			// Then
			Expect(err).ToNot(HaveOccurred())
			// The deleted event must not look it up again.
			Expect(sut.Runtime().HasRuntimeImpl(testContainer.ID())).To(BeFalse())
		})

		It("should not drop its records while the init process is still alive", func() {
			// Given a container whose init process is running, but whose state.Pid
			// was never populated because the runtime's state output was never read
			// successfully. Stopping and deleting both report success without
			// touching the process, so without the liveness gate in
			// removeContainerInPod this removal would erase every record of a
			// container that is still running, leaving nothing able to reap it.
			addContainerAndSandbox()

			cmd := exec.Command("sleep", "300")
			Expect(cmd.Start()).To(Succeed())

			defer func() {
				//nolint:errcheck // best-effort cleanup in test teardown
				cmd.Process.Kill()
				// make sure the entry in the process table is cleaned up
				//nolint:errcheck // best-effort cleanup in test teardown
				cmd.Wait()
			}()

			state := &oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateStopped},
			}
			Expect(state.SetInitPid(cmd.Process.Pid)).To(Succeed())
			state.Pid = 0
			testContainer.SetState(state)
			Expect(testContainer.InitProcessAlive()).To(BeTrue())

			runtimeServerMock.EXPECT().DeleteContainer(gomock.Any(), gomock.Any()).
				Return(nil).AnyTimes()
			testSandbox.SetStopped(context.Background(), false)

			// When
			_, err := sut.RemoveContainer(context.Background(),
				&types.RemoveContainerRequest{
					ContainerId: testContainer.ID(),
				})

			// Then the init process must not have outlived the removal. Do not
			// reap it first: a SIGKILLed process lingers as a zombie until the
			// deferred Wait above collects it, and InitProcessAlive already
			// treats a zombie as gone. Waiting here would instead block until
			// the process exited on its own and mask a missing kill entirely.
			Expect(err).ToNot(HaveOccurred())
			Expect(testContainer.InitProcessAlive()).To(BeFalse())
		})

		It("should succeed if container is not found", func() {
			// Given
			// When
			_, err := sut.RemoveContainer(context.Background(),
				&types.RemoveContainerRequest{ContainerId: "id"})

			// Then
			Expect(err).NotTo(HaveOccurred())
		})

		It("should fail on container remove error", func() {
			// Given
			// When
			_, err := sut.RemoveContainer(context.Background(),
				&types.RemoveContainerRequest{})

			// Then
			Expect(err).To(HaveOccurred())
		})
	})
})
