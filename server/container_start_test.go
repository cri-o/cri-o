package server_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
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
			_, err := sut.StartContainer(context.Background(),
				&types.StartContainerRequest{
					ContainerId: testContainer.ID(),
				})

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail with invalid container ID", func() {
			// Given
			// When
			_, err := sut.StartContainer(context.Background(),
				&types.StartContainerRequest{})

			// Then
			Expect(err).NotTo(BeNil())
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
			Expect(err).NotTo(BeNil())
		})
	})
})
