package server_test

import (
	"context"

	"github.com/cri-o/cri-o/oci"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// The actual test suite
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
			// When
			response, err := sut.PortForward(context.Background(),
				&pb.PortForwardRequest{
					PodSandboxId: testSandbox.ID(),
					Port:         []int32{33300},
				})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
		})

		It("should fail on missing sandbox ID", func() {
			// Given
			// When
			response, err := sut.PortForward(context.Background(),
				&pb.PortForwardRequest{})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})
	})

	t.Describe("StreamServer: PortForward", func() {
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
				ociRuntimeMock.EXPECT().PortForwardContainer(gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil),
			)

			// When
			err := testStreamService.PortForward(testSandbox.ID(), 0, nil)

			// Then
			Expect(err).To(BeNil())
		})

		It("shoud fail when PortForwardContainer errors", func() {
			// Given
			testStreamService.RuntimeServer().SetRuntime(ociRuntimeMock)
			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			addContainerAndSandboxRuntimeServer()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any()).
					Return(nil),
				ociRuntimeMock.EXPECT().PortForwardContainer(gomock.Any(),
					gomock.Any(), gomock.Any()).Return(t.TestError),
			)

			// When
			err := testStreamService.PortForward(testSandbox.ID(), 0, nil)

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
			err := testStreamService.PortForward(testSandbox.ID(), 0, nil)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("shoud fail when container is not running", func() {
			// Given
			testStreamService.RuntimeServer().SetRuntime(ociRuntimeMock)
			addContainerAndSandboxRuntimeServer()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any()).
					Return(nil),
			)

			// When
			err := testStreamService.PortForward(testSandbox.ID(), 0, nil)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("shoud fail when container is not available", func() {
			// Given
			testStreamService.RuntimeServer().SetRuntime(ociRuntimeMock)
			testStreamService.RuntimeServer().AddSandbox(testSandbox)
			Expect(testStreamService.RuntimeServer().PodIDIndex().
				Add(testSandbox.ID()))

			// When
			err := testStreamService.PortForward(testSandbox.ID(), 0, nil)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("shoud fail when sandbox not found", func() {
			// Given
			// When
			err := testStreamService.PortForward(testSandbox.ID(), 0, nil)

			// Then
			Expect(err).NotTo(BeNil())
		})
	})
})
