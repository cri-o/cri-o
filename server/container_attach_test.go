package server_test

import (
	"context"

	"github.com/cri-o/cri-o/internal/oci"
	publicOCI "github.com/cri-o/cri-o/pkg/oci"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"k8s.io/client-go/tools/remotecommand"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// The actual test suite
var _ = t.Describe("ContainerAttach", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("ContainerAttach", func() {
		It("should succeed", func() {
			// Given
			// When
			response, err := sut.Attach(context.Background(),
				&pb.AttachRequest{
					ContainerId: testContainer.ID(),
					Stdout:      true,
				})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
		})

		It("should fail on invalid request", func() {
			// Given
			// When
			response, err := sut.Attach(context.Background(),
				&pb.AttachRequest{})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})
	})

	t.Describe("StreamServer: Attach", func() {
		It("shoud succeed", func() {
			// Given
			testStreamService.RuntimeServer().SetRuntime(ociRuntimeMock)
			testContainer.SetState(&publicOCI.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			addContainerAndSandboxRuntimeServer()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any()).
					Return(nil),
				ociRuntimeMock.EXPECT().AttachContainer(gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any()).Return(nil),
			)

			// When
			err := testStreamService.Attach(testContainer.ID(),
				nil, nil, nil, false, make(chan remotecommand.TerminalSize))

			// Then
			Expect(err).To(BeNil())
		})

		It("shoud fail if container attach errors", func() {
			// Given
			testStreamService.RuntimeServer().SetRuntime(ociRuntimeMock)
			testContainer.SetState(&publicOCI.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			addContainerAndSandboxRuntimeServer()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any()).
					Return(nil),
				ociRuntimeMock.EXPECT().AttachContainer(gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any()).Return(t.TestError),
			)

			// When
			err := testStreamService.Attach(testContainer.ID(),
				nil, nil, nil, false, make(chan remotecommand.TerminalSize))

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("shoud fail if container is not running errors", func() {
			// Given
			testStreamService.RuntimeServer().SetRuntime(ociRuntimeMock)
			addContainerAndSandboxRuntimeServer()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any()).
					Return(nil),
			)

			// When
			err := testStreamService.Attach(testContainer.ID(),
				nil, nil, nil, false, make(chan remotecommand.TerminalSize))

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("shoud fail if container status update errors", func() {
			// Given
			testStreamService.RuntimeServer().SetRuntime(ociRuntimeMock)
			addContainerAndSandboxRuntimeServer()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any()).
					Return(t.TestError),
			)

			// When
			err := testStreamService.Attach(testContainer.ID(),
				nil, nil, nil, false, make(chan remotecommand.TerminalSize))

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("shoud fail if container was not found", func() {
			// Given
			// When
			err := testStreamService.Attach(testContainer.ID(),
				nil, nil, nil, false, make(chan remotecommand.TerminalSize))

			// Then
			Expect(err).NotTo(BeNil())
		})
	})
})
