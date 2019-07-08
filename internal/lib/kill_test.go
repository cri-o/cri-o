package lib_test

import (
	"syscall"

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

	t.Describe("ContainerKill", func() {
		It("should succeed", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			myContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			addContainerAndSandbox()

			gomock.InOrder(
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any()).
					Return(nil),
				ociRuntimeMock.EXPECT().SignalContainer(gomock.Any(),
					gomock.Any()).Return(nil),
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any()).
					Return(nil),
			)

			// When
			res, err := sut.ContainerKill(containerID, syscall.SIGINT)

			// Then
			Expect(err).To(BeNil())
			Expect(res).To(Equal(containerID))
		})

		It("should fail when not found", func() {
			// Given
			// When
			res, err := sut.ContainerKill("", syscall.SIGINT)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(Equal(""))
		})

		It("should fail when container is not running", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			addContainerAndSandbox()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any()).
					Return(nil),
			)

			// When
			res, err := sut.ContainerKill(containerID, syscall.SIGINT)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeEmpty())
		})

		It("should fail when container signaling errors", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			myContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			addContainerAndSandbox()

			gomock.InOrder(
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any()).
					Return(nil),
				ociRuntimeMock.EXPECT().SignalContainer(gomock.Any(),
					gomock.Any()).Return(t.TestError),
			)

			// When
			res, err := sut.ContainerKill(containerID, syscall.SIGINT)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeEmpty())
		})

	})
})
