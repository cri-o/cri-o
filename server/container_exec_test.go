package server_test

import (
	"context"
	"errors"
	"fmt"

	conmonClient "github.com/containers/conmon-rs/pkg/client"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/runtime-spec/specs-go"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	"k8s.io/cri-streaming/pkg/streaming/remotecommand"

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

	t.Describe("ContainerExec with conmon-rs", func() {
		const monitorURL = "http://127.0.0.1:1234/exec/token"

		// streamWebsockets lets the runtime handler of the test sandbox
		// serve exec sessions from the container monitor.
		streamWebsockets := func() {
			addContainerAndSandbox()

			handler := serverConfig.Runtimes[serverConfig.DefaultRuntime]
			handler.StreamWebsockets = true
			handler.SetConmonrsFeatures([]string{conmonClient.FeatureStreamingProtocols})

			// The server's background monitor may probe the container while
			// this test installs its runtime implementation.
			ociRuntimeMock.EXPECT().
				ProbeMonitor(gomock.Any(), testContainer).
				Return(nil).
				AnyTimes()
			sut.Runtime().SetRuntimeImpl(testContainer.ID(), ociRuntimeMock)
		}

		request := func() *types.ExecRequest {
			return &types.ExecRequest{
				ContainerId: testContainer.ID(),
				Stdout:      true,
			}
		}

		It("should use the URL of the container monitor", func() {
			// Given
			streamWebsockets()
			ociRuntimeMock.EXPECT().
				ServeExecContainer(gomock.Any(), testContainer, gomock.Any(), false, false, true, false).
				Return(monitorURL, nil)

			// When
			response, err := sut.Exec(context.Background(), request())

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response.GetUrl()).To(Equal(monitorURL))
		})

		It("should use the streaming server if the running monitor does not support it", func() {
			// Given
			streamWebsockets()
			ociRuntimeMock.EXPECT().
				ServeExecContainer(gomock.Any(), testContainer, gomock.Any(), false, false, true, false).
				Return("", fmt.Errorf("conmon-rs server: %w", errors.ErrUnsupported))

			// When
			response, err := sut.Exec(context.Background(), request())

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response.GetUrl()).NotTo(BeEmpty())
			Expect(response.GetUrl()).NotTo(Equal(monitorURL))
		})

		It("should fail with ResourceExhausted if too many URLs wait", func() {
			// Given
			streamWebsockets()
			ociRuntimeMock.EXPECT().
				ServeExecContainer(gomock.Any(), testContainer, gomock.Any(), false, false, true, false).
				Return("", fmt.Errorf("call ServeExecContainer RPC: %w", conmonClient.ErrTooManyRequests))

			// When
			response, err := sut.Exec(context.Background(), request())

			// Then
			Expect(status.Code(err)).To(Equal(codes.ResourceExhausted))
			Expect(response).To(BeNil())
		})

		It("should fail if the container monitor fails", func() {
			// Given
			streamWebsockets()
			ociRuntimeMock.EXPECT().
				ServeExecContainer(gomock.Any(), testContainer, gomock.Any(), false, false, true, false).
				Return("", errors.New("error"))

			// When
			response, err := sut.Exec(context.Background(), request())

			// Then
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.Unknown))
			Expect(response).To(BeNil())
		})
	})

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
			err := testStreamService.Exec(
				context.Background(),
				testContainer.ID(),
				[]string{"/bin/sh"},
				nil,
				nil,
				nil,
				false,
				make(chan remotecommand.TerminalSize),
			)

			// Then - Should succeed because container is running
			Expect(
				err,
			).To(HaveOccurred())
			// Will fail trying to actually exec but won't fail the Living() check
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
			err := testStreamService.Exec(
				context.Background(),
				testContainer.ID(),
				[]string{"/bin/sh"},
				nil,
				nil,
				nil,
				false,
				make(chan remotecommand.TerminalSize),
			)

			// Then - Should fail the Living() check
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("container is not created or running"))
		})
	})
})
