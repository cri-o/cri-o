package server_test

import (
	"context"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/runtime-spec/specs-go"
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
		It("shoud succeed", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			addContainerAndSandbox()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any()).
					Return(nil),
				ociRuntimeMock.EXPECT().ExecSyncContainer(
					gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&oci.ExecSyncResponse{}, nil),
			)

			// When
			response, err := sut.ExecSync(context.Background(),
				&pb.ExecSyncRequest{
					ContainerId: testContainer.ID(),
					Cmd:         []string{},
				})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
		})

		It("shoud fail if exec sync erros", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			addContainerAndSandbox()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any()).
					Return(nil),
				ociRuntimeMock.EXPECT().ExecSyncContainer(
					gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, t.TestError),
			)

			// When
			response, err := sut.ExecSync(context.Background(),
				&pb.ExecSyncRequest{
					ContainerId: testContainer.ID(),
					Cmd:         []string{},
				})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("shoud fail if command is nil", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			addContainerAndSandbox()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any()).
					Return(nil),
			)

			// When
			response, err := sut.ExecSync(context.Background(),
				&pb.ExecSyncRequest{ContainerId: testContainer.ID()})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("shoud fail if container status invalid", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateStopped},
			})
			addContainerAndSandbox()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any()).
					Return(nil),
			)

			// When
			response, err := sut.ExecSync(context.Background(),
				&pb.ExecSyncRequest{ContainerId: testContainer.ID()})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("shoud fail if container status update erros", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateStopped},
			})
			addContainerAndSandbox()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any()).
					Return(t.TestError),
			)

			// When
			response, err := sut.ExecSync(context.Background(),
				&pb.ExecSyncRequest{ContainerId: testContainer.ID()})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail with invalid container ID", func() {
			// Given
			// When
			response, err := sut.ExecSync(context.Background(),
				&pb.ExecSyncRequest{})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})
	})
})
