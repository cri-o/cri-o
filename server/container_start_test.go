package server_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"go.uber.org/mock/gomock"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/oci"
)

// The actual test suite.
var _ = t.Describe("ContainerStart", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("ContainerStart", func() {
		It("should fail with container not in created state", func() {
			// Given
			addContainerAndSandbox()

			// When
			_, err := sut.StartContainer(context.Background(),
				&types.StartContainerRequest{
					ContainerId: testContainer.ID(),
				})

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail with invalid container ID", func() {
			// Given
			// When
			_, err := sut.StartContainer(context.Background(),
				&types.StartContainerRequest{})

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should report a container which failed to start as exited", func() {
			// Given
			addContainerAndSandbox()
			testContainer.SetState(&oci.ContainerState{
				State:    specs.State{Status: oci.ContainerStateCreated},
				ExitCode: new(int32(0)),
			})
			sut.Runtime().SetRuntimeImplForContainer(testContainer, ociRuntimeMock)
			DeferCleanup(func() {
				sut.Runtime().SetRuntimeImplForContainer(testContainer, nil)
			})
			ociRuntimeMock.EXPECT().ProbeMonitor(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().StartContainer(gomock.Any(), testContainer).
					DoAndReturn(func(context.Context, *oci.Container) error {
						// The runtime may already report the container as stopped.
						testContainer.SetState(&oci.ContainerState{
							State:    specs.State{Status: oci.ContainerStateStopped},
							ExitCode: new(int32(1)),
						})

						return errors.New("start failed")
					}),
				// The cleanup removal fails, so the container stays visible to CRI.
				ociRuntimeMock.EXPECT().StopContainer(gomock.Any(), testContainer, gomock.Any()).
					Return(errors.New("stop failed")),
			)

			// When
			_, err := sut.StartContainer(context.Background(),
				&types.StartContainerRequest{
					ContainerId: testContainer.ID(),
				})

			// Then
			Expect(err).To(HaveOccurred())
			response, err := sut.ContainerStatus(context.Background(),
				&types.ContainerStatusRequest{ContainerId: testContainer.ID()})
			Expect(err).ToNot(HaveOccurred())
			Expect(response.GetStatus().GetState()).To(Equal(types.ContainerState_CONTAINER_EXITED))
		})

		It("should fail with invalid container state", func() {
			// Given
			addContainerAndSandbox()

			// When
			_, err := sut.StartContainer(context.Background(),
				&types.StartContainerRequest{
					ContainerId: testContainer.ID(),
				},
			)

			// Then
			Expect(err).To(HaveOccurred())
		})
	})
})
