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

	"github.com/cri-o/cri-o/pkg/config"
)

// The actual test suite.
var _ = t.Describe("ContainerPortforward", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("ContainerPortforward", func() {
		It("should succeed", func() {
			// Given
			addContainerAndSandbox()

			// When
			response, err := sut.PortForward(context.Background(),
				&types.PortForwardRequest{
					PodSandboxId: testSandbox.ID(),
					Port:         []int32{33300},
				})

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response).NotTo(BeNil())
		})

		It("should fail on missing sandbox ID", func() {
			// Given
			// When
			response, err := sut.PortForward(context.Background(),
				&types.PortForwardRequest{})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(response).To(BeNil())
		})

		It("should fail on unknown sandbox", func() {
			// Given
			// When
			response, err := sut.PortForward(context.Background(),
				&types.PortForwardRequest{PodSandboxId: testSandbox.ID()})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(response).To(BeNil())
		})
	})

	t.Describe("ContainerPortforward with conmon-rs", func() {
		const monitorURL = "http://127.0.0.1:1234/portforward/token"

		// streamPortForward lets the runtime handler of the test sandbox
		// serve port forwarding from the container monitor if the features
		// contain it.
		streamPortForward := func(features ...string) {
			addContainerAndSandbox()
			Expect(testSandbox.NetNsJoin("/proc/self/ns/net")).To(Succeed())

			handler := serverConfig.Runtimes[serverConfig.DefaultRuntime]
			handler.StreamWebsockets = true
			handler.SetConmonrsFeatures(features)

			// The server's background monitor may probe the container while
			// this test installs its runtime implementation.
			ociRuntimeMock.EXPECT().
				ProbeMonitor(gomock.Any(), testContainer).
				Return(nil).
				AnyTimes()
			sut.Runtime().SetRuntimeImpl(testContainer.ID(), ociRuntimeMock)
		}

		request := func() *types.PortForwardRequest {
			return &types.PortForwardRequest{
				PodSandboxId: testSandbox.ID(),
				Port:         []int32{33300},
			}
		}

		It("should use the URL of the container monitor", func() {
			// Given
			streamPortForward(conmonClient.FeaturePortForward)
			ociRuntimeMock.EXPECT().
				ServePortForwardContainer(gomock.Any(), testContainer, testSandbox.NetNsPath()).
				Return(monitorURL, nil)

			// When
			response, err := sut.PortForward(context.Background(), request())

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response.GetUrl()).To(Equal(monitorURL + "?port=33300"))
		})

		It("should pass all ports to the container monitor", func() {
			// Given
			streamPortForward(conmonClient.FeaturePortForward)
			ociRuntimeMock.EXPECT().
				ServePortForwardContainer(gomock.Any(), testContainer, gomock.Any()).
				Return(monitorURL, nil)

			req := request()
			req.Port = []int32{80, 8080}

			// When
			response, err := sut.PortForward(context.Background(), req)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response.GetUrl()).To(Equal(monitorURL + "?port=80&port=8080"))
		})

		It("should keep the URL of the container monitor without ports", func() {
			// Given
			streamPortForward(conmonClient.FeaturePortForward)
			ociRuntimeMock.EXPECT().
				ServePortForwardContainer(gomock.Any(), testContainer, gomock.Any()).
				Return(monitorURL, nil)

			req := request()
			req.Port = nil

			// When
			response, err := sut.PortForward(context.Background(), req)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response.GetUrl()).To(Equal(monitorURL))
		})

		It("should use the streaming server without the port forward feature", func() {
			// Given
			streamPortForward(conmonClient.FeatureStreamingProtocols)

			// When
			response, err := sut.PortForward(context.Background(), request())

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response.GetUrl()).NotTo(BeEmpty())
			Expect(response.GetUrl()).NotTo(Equal(monitorURL))
		})

		It("should use the streaming server if the running monitor does not support it", func() {
			// Given
			streamPortForward(conmonClient.FeaturePortForward)
			ociRuntimeMock.EXPECT().
				ServePortForwardContainer(gomock.Any(), testContainer, gomock.Any()).
				Return("", fmt.Errorf("call ServePortForwardContainer RPC: %w", errors.ErrUnsupported))

			// When
			response, err := sut.PortForward(context.Background(), request())

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response.GetUrl()).NotTo(BeEmpty())
			Expect(response.GetUrl()).NotTo(Equal(monitorURL))
		})

		// enablePortForward lets the runtime handler of the test sandbox
		// serve port forwarding from the container monitor without
		// installing a runtime implementation for its infra container.
		enablePortForward := func() {
			addContainerAndSandbox()
			Expect(testSandbox.NetNsJoin("/proc/self/ns/net")).To(Succeed())

			handler := serverConfig.Runtimes[serverConfig.DefaultRuntime]
			Expect(handler.RuntimeType).To(Equal(config.DefaultRuntimeType))
			handler.StreamWebsockets = true
			handler.SetConmonrsFeatures([]string{conmonClient.FeaturePortForward})
		}

		It("should not create a runtime for a removed sandbox", func() {
			// Given
			// The sandbox got removed after the request found it, together
			// with the runtime implementation of its infra container.
			enablePortForward()

			// When
			response, err := sut.PortForward(context.Background(), request())

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response.GetUrl()).NotTo(BeEmpty())
			Expect(response.GetUrl()).NotTo(Equal(monitorURL))
			Expect(sut.Runtime().HasRuntimeImpl(testContainer.ID())).To(BeFalse())
		})

		It("should not create a runtime for a stopping sandbox", func() {
			// Given
			enablePortForward()
			testSandbox.SetStopped(context.Background(), false)

			// When
			response, err := sut.PortForward(context.Background(), request())

			// Then
			Expect(err).To(MatchError(ContainSubstring("is not running")))
			Expect(response).To(BeNil())
			Expect(sut.Runtime().HasRuntimeImpl(testContainer.ID())).To(BeFalse())
		})

		It("should fail with ResourceExhausted if too many URLs wait", func() {
			// Given
			streamPortForward(conmonClient.FeaturePortForward)
			ociRuntimeMock.EXPECT().
				ServePortForwardContainer(gomock.Any(), testContainer, gomock.Any()).
				Return("", fmt.Errorf("call ServePortForwardContainer RPC: %w", conmonClient.ErrTooManyRequests))

			// When
			response, err := sut.PortForward(context.Background(), request())

			// Then
			Expect(status.Code(err)).To(Equal(codes.ResourceExhausted))
			Expect(response).To(BeNil())
		})

		It("should fail if the container monitor fails", func() {
			// Given
			streamPortForward(conmonClient.FeaturePortForward)
			ociRuntimeMock.EXPECT().
				ServePortForwardContainer(gomock.Any(), testContainer, gomock.Any()).
				Return("", errors.New("error"))

			// When
			response, err := sut.PortForward(context.Background(), request())

			// Then
			Expect(err).To(HaveOccurred())
			Expect(response).To(BeNil())
		})

		It("should fail without a network namespace", func() {
			// Given
			addContainerAndSandbox()

			handler := serverConfig.Runtimes[serverConfig.DefaultRuntime]
			handler.StreamWebsockets = true
			handler.SetConmonrsFeatures([]string{conmonClient.FeaturePortForward})

			// When
			response, err := sut.PortForward(context.Background(), request())

			// Then
			Expect(err).To(HaveOccurred())
			Expect(response).To(BeNil())
		})
	})

	t.Describe("StreamServer: PortForward", func() {
		It("should fail when sandbox not found", func() {
			// Given
			// When
			err := testStreamService.PortForward(context.Background(), testSandbox.ID(), 0, nil)

			// Then
			Expect(err).To(HaveOccurred())
		})
	})
})
