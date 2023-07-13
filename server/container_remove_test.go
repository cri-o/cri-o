package server_test

import (
	"context"

	"github.com/cri-o/cri-o/internal/oci"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"go.uber.org/mock/gomock"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// The actual test suite
var _ = t.Describe("ContainerRemove", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		mockRuncInLibConfig()
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
				runtimeServerMock.EXPECT().StopContainer(gomock.Any(), gomock.Any()).
					Return(nil),
				runtimeServerMock.EXPECT().DeleteContainer(gomock.Any(), gomock.Any()).
					Return(nil),
			)

			// When
			_, err := sut.RemoveContainer(context.Background(),
				&types.RemoveContainerRequest{
					ContainerId: testContainer.ID(),
				})

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail on container remove error", func() {
			// Given
			// When
			_, err := sut.RemoveContainer(context.Background(),
				&types.RemoveContainerRequest{})

			// Then
			Expect(err).NotTo(BeNil())
		})
	})
})
