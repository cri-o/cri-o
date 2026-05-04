package server_test

import (
	"context"

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

		It("should wait for a running post-stop cleanup in a stopped sandbox", func() {
			// Given
			addContainerAndSandbox()
			testContainer.SetState(&oci.ContainerState{
				State:    specs.State{Status: oci.ContainerStateStopped},
				ExitCode: new(int32(0)),
			})
			testSandbox.SetStopped(context.Background(), false)
			sut.Runtime().SetRuntimeImplForContainer(testContainer, ociRuntimeMock)
			DeferCleanup(func() {
				sut.Runtime().SetRuntimeImplForContainer(testContainer, nil)
			})

			started := make(chan struct{})
			release := make(chan struct{})

			go testContainer.RunPostStopCleanup(func() {
				close(started)
				<-release
			})
			<-started

			deleted := make(chan struct{})
			ociRuntimeMock.EXPECT().ProbeMonitor(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().DeleteContainer(gomock.Any(), testContainer).
					DoAndReturn(func(context.Context, *oci.Container) error {
						close(deleted)

						return nil
					}),
				runtimeServerMock.EXPECT().DeleteContainer(gomock.Any(), gomock.Any()).
					Return(nil),
			)

			// When
			removed := make(chan error, 1)

			go func() {
				_, err := sut.RemoveContainer(context.Background(),
					&types.RemoveContainerRequest{ContainerId: testContainer.ID()})
				removed <- err
			}()

			// Then
			Consistently(deleted).ShouldNot(BeClosed())

			// When
			close(release)

			// Then
			Eventually(removed).Should(Receive(BeNil()))
			Expect(deleted).To(BeClosed())
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
