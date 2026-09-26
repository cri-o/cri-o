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
var _ = t.Describe("ContainerStop", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(func() {
		if sut != nil {
			sut.Runtime().SetRuntimeImplForContainer(testContainer, nil)
		}

		afterEach()
	})

	t.Describe("ContainerStop", func() {
		It("should succeed even if runtime DeleteContainer fails", func() {
			// Given
			addContainerAndSandbox()
			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateStopped},
			})
			sut.Runtime().SetRuntimeImplForContainer(testContainer, ociRuntimeMock)
			ociRuntimeMock.EXPECT().ProbeMonitor(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().
					StopContainer(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil),
				runtimeServerMock.EXPECT().StopContainer(gomock.Any(), gomock.Any()).Return(nil),
				ociRuntimeMock.EXPECT().
					UpdateContainerStatus(gomock.Any(), gomock.Any()).
					Return(nil),
				ociRuntimeMock.EXPECT().DeleteContainer(gomock.Any(), gomock.Any()).
					Return(errors.New("boom")),
			)

			// When
			_, err := sut.StopContainer(context.Background(),
				&types.StopContainerRequest{
					ContainerId: testContainer.ID(),
				})

			// Then
			Expect(err).ToNot(HaveOccurred())
		})
		It("should succeed", func() {
			// Given
			addContainerAndSandbox()
			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateStopped},
			})
			sut.Runtime().SetRuntimeImplForContainer(testContainer, ociRuntimeMock)
			ociRuntimeMock.EXPECT().ProbeMonitor(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().StopContainer(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil),
				runtimeServerMock.EXPECT().StopContainer(gomock.Any(), gomock.Any()).
					Return(nil),
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any(), gomock.Any()).
					Return(nil),
				ociRuntimeMock.EXPECT().DeleteContainer(gomock.Any(), gomock.Any()).
					Return(nil),
			)

			// When
			_, err := sut.StopContainer(context.Background(),
				&types.StopContainerRequest{
					ContainerId: testContainer.ID(),
				})

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should retain the runtime implementation until container removal", func() {
			// Given
			addContainerAndSandbox()
			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateStopped},
			})
			testSandbox.SetStopped(context.Background(), true)
			sut.Runtime().SetRuntimeImplForContainer(testContainer, ociRuntimeMock)
			ociRuntimeMock.EXPECT().ProbeMonitor(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().
					StopContainer(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil),
				runtimeServerMock.EXPECT().StopContainer(gomock.Any(), gomock.Any()).Return(nil),
				ociRuntimeMock.EXPECT().
					UpdateContainerStatus(gomock.Any(), gomock.Any()).
					Return(nil),
				ociRuntimeMock.EXPECT().DeleteContainer(gomock.Any(), gomock.Any()).Return(nil),
				ociRuntimeMock.EXPECT().DeleteContainer(gomock.Any(), gomock.Any()).Return(nil),
				runtimeServerMock.EXPECT().DeleteContainer(gomock.Any(), gomock.Any()).Return(nil),
			)

			// When
			_, err := sut.StopContainer(
				context.Background(),
				&types.StopContainerRequest{ContainerId: testContainer.ID()},
			)
			Expect(err).ToNot(HaveOccurred())
			_, err = sut.RemoveContainer(
				context.Background(),
				&types.RemoveContainerRequest{ContainerId: testContainer.ID()},
			)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should skip runtime deletion when the implementation is absent", func() {
			// Given
			addContainerAndSandbox()
			sut.Runtime().SetRuntimeImplForContainer(testContainer, nil)

			// When
			err := sut.Runtime().DeleteRuntimeContainer(context.Background(), testContainer)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should succeed with not existing container ID", func() {
			// Given
			// When
			_, err := sut.StopContainer(context.Background(),
				&types.StopContainerRequest{ContainerId: "id"})

			// Then
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
