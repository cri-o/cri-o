package lib_test

import (
	"context"

	"github.com/containers/storage"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/runtime-spec/specs-go"
)

// The actual test suite
var _ = t.Describe("ContainerServer", func() {
	// Prepare the sut
	BeforeEach(beforeEach)

	t.Describe("Stop", func() {
		It("should succeed", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			myContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			addContainerAndSandbox()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().StopContainer(gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil),
				ociRuntimeMock.EXPECT().WaitContainerStateStopped(
					gomock.Any(), gomock.Any()).Return(nil),
				storeMock.EXPECT().Container(gomock.Any()).
					Return(&storage.Container{}, nil),
				storeMock.EXPECT().Unmount(gomock.Any(), gomock.Any()).
					Return(true, nil),
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any()).
					Return(nil),
			)

			// When
			res, err := sut.ContainerStop(context.Background(), containerID, 0)

			// Then
			Expect(err).To(BeNil())
			Expect(res).To(Equal(containerID))
		})

		It("should fail when wait for container state stopped errors", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			myContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			addContainerAndSandbox()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().StopContainer(gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil),
				ociRuntimeMock.EXPECT().WaitContainerStateStopped(
					gomock.Any(), gomock.Any()).Return(t.TestError),
			)

			// When
			res, err := sut.ContainerStop(context.Background(), containerID, 0)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeEmpty())
		})

		It("should fail when storage stop fails", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			myContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			addContainerAndSandbox()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().StopContainer(gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil),
				ociRuntimeMock.EXPECT().WaitContainerStateStopped(
					gomock.Any(), gomock.Any()).Return(nil),
				storeMock.EXPECT().Container(gomock.Any()).
					Return(nil, t.TestError),
			)

			// When
			res, err := sut.ContainerStop(context.Background(), containerID, 0)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeEmpty())
		})

		It("should fail when runtime stop fails", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			myContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			addContainerAndSandbox()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().StopContainer(gomock.Any(),
					gomock.Any(), gomock.Any()).Return(t.TestError),
			)

			// When
			res, err := sut.ContainerStop(context.Background(), containerID, 0)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeEmpty())
		})

		It("should fail container paused", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			myContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStatePaused},
			})
			addContainerAndSandbox()

			// When
			res, err := sut.ContainerStop(context.Background(), containerID, 0)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeEmpty())
		})

		It("should fail on invalid container ID", func() {
			// Given
			// When
			res, err := sut.ContainerStop(context.Background(), "", 0)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeEmpty())
		})
	})
})
