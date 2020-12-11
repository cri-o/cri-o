package server_test

import (
	"context"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/server/cri/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

// The actual test suite
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

			// When
			err := sut.StopContainer(context.Background(),
				&types.StopContainerRequest{
					ContainerID: testContainer.ID(),
				})

			// Then
			Expect(err).To(BeNil())
		})

		It("should afil with invalid container id", func() {
			// Given
			// When
			err := sut.StopContainer(context.Background(),
				&types.StopContainerRequest{ContainerID: "id"})

			// Then
			Expect(err).NotTo(BeNil())
		})
	})
})
