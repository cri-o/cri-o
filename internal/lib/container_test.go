package lib_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("ContainerServer", func() {
	ctx := context.TODO()
	// Prepare the sut
	BeforeEach(beforeEach)

	t.Describe("LookupContainer", func() {
		It("should succeed", func() {
			// Given
			addContainerAndSandbox()

			// When
			container, err := sut.LookupContainer(ctx, containerID)

			// Then
			Expect(err).To(BeNil())
			Expect(container).NotTo(BeNil())
		})

		It("should fail with empty ID", func() {
			// Given

			// When
			container, err := sut.LookupContainer(ctx, "")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(container).To(BeNil())
		})
	})

	t.Describe("GetContainerFromShortID", func() {
		It("should succeed", func() {
			// Given
			addContainerAndSandbox()

			// When
			container, err := sut.GetContainerFromShortID(ctx, containerID)

			// Then
			Expect(err).To(BeNil())
			Expect(container).NotTo(BeNil())
		})

		It("should fail with empty ID", func() {
			// Given

			// When
			container, err := sut.GetContainerFromShortID(ctx, "")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(container).To(BeNil())
		})

		It("should fail with invalid ID", func() {
			// Given

			// When
			container, err := sut.GetContainerFromShortID(ctx, "invalid")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(container).To(BeNil())
		})

		It("should fail if container is not created", func() {
			ctx := context.TODO()
			// Given
			Expect(sut.AddSandbox(ctx, mySandbox)).To(BeNil())
			sut.AddContainer(ctx, myContainer)
			Expect(sut.CtrIDIndex().Add(containerID)).To(BeNil())
			Expect(sut.PodIDIndex().Add(sandboxID)).To(BeNil())

			// When
			container, err := sut.GetContainerFromShortID(ctx, containerID)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(container).To(BeNil())
		})
	})
})
