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
var _ = t.Describe("ContainerStop", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("ContainerStop", func() {
		It("should succeed", func() {
			// Given
			addContainerAndSandbox()
			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateStopped},
			})
			gomock.InOrder(
				runtimeServerMock.EXPECT().StopContainer(gomock.Any(), gomock.Any()).
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
