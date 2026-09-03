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

		It("should restore a container without runtime status", func() {
			// Given
			addContainerAndSandbox()
			testContainer.SetRestore(true)

			// The restore fails without a checkpoint, which stops the container
			// for removal.
			runtimeServerMock.EXPECT().StopContainer(gomock.Any(), gomock.Any()).
				Return(nil)

			// When
			_, err := sut.StartContainer(context.Background(),
				&types.StartContainerRequest{
					ContainerId: testContainer.ID(),
				})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).NotTo(ContainSubstring("not in created state"))
		})

		It("should fail to restore a container in another state", func() {
			// Given
			addContainerAndSandbox()
			testContainer.SetRestore(true)
			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})

			// When
			_, err := sut.StartContainer(context.Background(),
				&types.StartContainerRequest{
					ContainerId: testContainer.ID(),
				})

			// Then
			Expect(err).To(MatchError(ContainSubstring("not in created state")))
		})

		It("should fail with invalid container ID", func() {
			// Given
			// When
			_, err := sut.StartContainer(context.Background(),
				&types.StartContainerRequest{})

			// Then
			Expect(err).To(HaveOccurred())
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
