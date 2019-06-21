package lib_test

import (
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

	t.Describe("ContainerPause", func() {
		It("should succeed", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			myContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			addContainerAndSandbox()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().PauseContainer(gomock.Any()).
					Return(nil),
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any()).
					Return(nil),
			)

			// When
			res, err := sut.ContainerPause(containerID)

			// Then
			Expect(err).To(BeNil())
			Expect(res).To(Equal(containerID))
		})

		It("should fail when container pause errors", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			myContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			addContainerAndSandbox()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().PauseContainer(gomock.Any()).
					Return(t.TestError),
			)

			// When
			res, err := sut.ContainerPause(containerID)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeEmpty())
		})

		It("should fail when already paused", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			myContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStatePaused},
			})
			addContainerAndSandbox()

			// When
			res, err := sut.ContainerPause(containerID)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeEmpty())
		})

		It("should fail with invalid container ID", func() {
			// Given

			// When
			res, err := sut.ContainerPause("")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(Equal(""))
		})
	})

	t.Describe("ContainerUnpause", func() {
		It("should succeed", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			myContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStatePaused},
			})
			addContainerAndSandbox()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().UnpauseContainer(gomock.Any()).
					Return(nil),
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any()).
					Return(nil),
			)

			// When
			res, err := sut.ContainerUnpause(containerID)

			// Then
			Expect(err).To(BeNil())
			Expect(res).To(Equal(containerID))
		})

		It("should fail when container unpause errors", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			myContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStatePaused},
			})
			addContainerAndSandbox()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().UnpauseContainer(gomock.Any()).
					Return(t.TestError),
			)

			// When
			res, err := sut.ContainerUnpause(containerID)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeEmpty())
		})

		It("should fail when container not paused", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			addContainerAndSandbox()

			// When
			res, err := sut.ContainerUnpause(containerID)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeEmpty())
		})

		It("should fail on invalid container", func() {
			// Given
			// When
			res, err := sut.ContainerUnpause("")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeEmpty())
		})
	})
})
