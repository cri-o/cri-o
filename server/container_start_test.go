package server_test

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// The actual test suite
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
			err := sut.StartContainer(context.Background(),
				&types.StartContainerRequest{
					ContainerID: testContainer.ID(),
				})

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail with invalid container ID", func() {
			// Given
			// When
			err := sut.StartContainer(context.Background(),
				&types.StartContainerRequest{})

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail with invalid container state", func() {
			// Given
			addContainerAndSandbox()

			// When
			err := sut.StartContainer(context.Background(),
				&types.StartContainerRequest{
					ContainerID: testContainer.ID(),
				},
			)

			// Then
			Expect(err).NotTo(BeNil())
		})
	})
})
