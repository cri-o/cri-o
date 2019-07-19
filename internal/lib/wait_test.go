package lib_test

import (
	"github.com/cri-o/cri-o/internal/oci"
	publicOCI "github.com/cri-o/cri-o/pkg/oci"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/runtime-spec/specs-go"
)

// The actual test suite
var _ = t.Describe("ContainerServer", func() {
	// Prepare the sut
	BeforeEach(beforeEach)

	t.Describe("ContainerWait", func() {
		It("should succeed", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			myContainer.SetState(&publicOCI.ContainerState{
				State: specs.State{Status: oci.ContainerStateStopped},
			})
			addContainerAndSandbox()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any()).
					Times(2).Return(nil),
			)

			// When
			res, err := sut.ContainerWait(containerID)

			// Then
			Expect(err).To(BeNil())
			Expect(res).To(BeZero())
		})

		It("should fail on invalid container ID", func() {
			// Given
			// When
			res, err := sut.ContainerWait("")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeEquivalentTo(0))
		})
	})
})
