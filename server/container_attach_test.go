package server_test

import (
	"context"
	"errors"
	"fmt"

	conmonClient "github.com/containers/conmon-rs/pkg/client"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	"k8s.io/cri-streaming/pkg/streaming/remotecommand"
)

// The actual test suite.
var _ = t.Describe("ContainerAttach", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("ContainerAttach with conmon-rs", func() {
		const monitorURL = "http://127.0.0.1:1234/attach/token"

		// streamWebsockets lets the runtime handler of the test sandbox
		// serve attach sessions from the container monitor.
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

		request := func() *types.AttachRequest {
			return &types.AttachRequest{
				ContainerId: testContainer.ID(),
				Stdout:      true,
			}
		}

		It("should use the URL of the container monitor", func() {
			// Given
			streamWebsockets()
			ociRuntimeMock.EXPECT().
				ServeAttachContainer(gomock.Any(), testContainer, false, true, false).
				Return(monitorURL, nil)

			// When
			response, err := sut.Attach(context.Background(), request())

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response.GetUrl()).To(Equal(monitorURL))
		})

		It("should use the streaming server if the running monitor does not support it", func() {
			// Given
			streamWebsockets()
			ociRuntimeMock.EXPECT().
				ServeAttachContainer(gomock.Any(), testContainer, false, true, false).
				Return("", fmt.Errorf("conmon-rs server: %w", errors.ErrUnsupported))

			// When
			response, err := sut.Attach(context.Background(), request())

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response.GetUrl()).NotTo(BeEmpty())
			Expect(response.GetUrl()).NotTo(Equal(monitorURL))
		})

		It("should fail with ResourceExhausted if too many URLs wait", func() {
			// Given
			streamWebsockets()
			ociRuntimeMock.EXPECT().
				ServeAttachContainer(gomock.Any(), testContainer, false, true, false).
				Return("", fmt.Errorf("call ServeAttachContainer RPC: %w", conmonClient.ErrTooManyRequests))

			// When
			response, err := sut.Attach(context.Background(), request())

			// Then
			Expect(status.Code(err)).To(Equal(codes.ResourceExhausted))
			Expect(response).To(BeNil())
		})

		It("should fail if the container monitor fails", func() {
			// Given
			streamWebsockets()
			ociRuntimeMock.EXPECT().
				ServeAttachContainer(gomock.Any(), testContainer, false, true, false).
				Return("", errors.New("error"))

			// When
			response, err := sut.Attach(context.Background(), request())

			// Then
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.Unknown))
			Expect(response).To(BeNil())
		})
	})

	t.Describe("ContainerAttach", func() {
		It("should succeed", func() {
			// Given
			addContainerAndSandbox()

			// When
			response, err := sut.Attach(context.Background(),
				&types.AttachRequest{
					ContainerId: testContainer.ID(),
					Stdout:      true,
				})

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response).NotTo(BeNil())
		})

		It("should include the error of the streaming server", func() {
			// Given
			addContainerAndSandbox()

			// When
			response, err := sut.Attach(context.Background(),
				&types.AttachRequest{ContainerId: testContainer.ID()})

			// Then
			Expect(err).To(MatchError(ContainSubstring("unable to prepare attach endpoint: ")))
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
			Expect(response).To(BeNil())
		})

		It("should fail on invalid request", func() {
			// Given
			// When
			response, err := sut.Attach(context.Background(),
				&types.AttachRequest{})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(response).To(BeNil())
		})
	})

	t.Describe("StreamServer: Attach", func() {
		It("should fail if container was not found", func() {
			// Given
			// When
			err := testStreamService.Attach(context.Background(), testContainer.ID(),
				nil, nil, nil, false, make(chan remotecommand.TerminalSize))

			// Then
			Expect(err).To(HaveOccurred())
		})
	})
})
