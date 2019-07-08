package server_test

import (
	"context"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"k8s.io/client-go/tools/remotecommand"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// The actual test suite
var _ = t.Describe("ContainerStart", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("ContainerStart", func() {
		It("should succeed", func() {
			// Given
			// When
			response, err := sut.Exec(context.Background(),
				&pb.ExecRequest{
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
			response, err := sut.Exec(context.Background(),
				&pb.ExecRequest{})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})
	})

	t.Describe("StreamServer: Exec", func() {
		It("shoud succeed", func() {
			// Given
			testStreamService.RuntimeServer().SetRuntime(ociRuntimeMock)
			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			addContainerAndSandboxRuntimeServer()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any()).
					Return(nil),
				ociRuntimeMock.EXPECT().ExecContainer(gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil),
			)

			// When
			err := testStreamService.Exec(testContainer.ID(), []string{},
				nil, nil, nil, false, make(chan remotecommand.TerminalSize))

			// Then
			Expect(err).To(BeNil())
		})

		It("shoud fail when ExecContainer errors", func() {
			// Given
			testStreamService.RuntimeServer().SetRuntime(ociRuntimeMock)
			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			addContainerAndSandboxRuntimeServer()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any()).
					Return(nil),
				ociRuntimeMock.EXPECT().ExecContainer(gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any()).Return(t.TestError),
			)

			// When
			err := testStreamService.Exec(testContainer.ID(), []string{},
				nil, nil, nil, false, make(chan remotecommand.TerminalSize))

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("shoud fail when container not running", func() {
			// Given
			testStreamService.RuntimeServer().SetRuntime(ociRuntimeMock)
			addContainerAndSandboxRuntimeServer()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any()).
					Return(nil),
			)

			// When
			err := testStreamService.Exec(testContainer.ID(), []string{},
				nil, nil, nil, false, make(chan remotecommand.TerminalSize))

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("shoud fail when container status update errors", func() {
			// Given
			testStreamService.RuntimeServer().SetRuntime(ociRuntimeMock)
			addContainerAndSandboxRuntimeServer()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any()).
					Return(t.TestError),
			)

			// When
			err := testStreamService.Exec(testContainer.ID(), []string{},
				nil, nil, nil, false, make(chan remotecommand.TerminalSize))

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("shoud fail when container not found", func() {
			// Given
			// When
			err := testStreamService.Exec(testContainer.ID(), []string{},
				nil, nil, nil, false, make(chan remotecommand.TerminalSize))

			// Then
			Expect(err).NotTo(BeNil())
		})
	})
})
