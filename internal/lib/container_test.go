package lib_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("ContainerServer", func() {
	// Prepare the sut
	BeforeEach(beforeEach)

	t.Describe("LookupContainer", func() {
		It("should succeed", func() {
			// Given
			addContainerAndSandbox()

			// When
			container, err := sut.LookupContainer(containerID)

			// Then
			Expect(err).To(BeNil())
			Expect(container).NotTo(BeNil())
		})

		It("should fail with empty ID", func() {
			// Given

			// When
			container, err := sut.LookupContainer("")

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
			container, err := sut.GetContainerFromShortID(containerID)

			// Then
			Expect(err).To(BeNil())
			Expect(container).NotTo(BeNil())
		})

		It("should fail with empty ID", func() {
			// Given

			// When
			container, err := sut.GetContainerFromShortID("")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(container).To(BeNil())
		})

		It("should fail with invalid ID", func() {
			// Given

			// When
			container, err := sut.GetContainerFromShortID("invalid")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(container).To(BeNil())
		})

		It("should fail if container is not created", func() {
			// Given
			Expect(sut.AddSandbox(mySandbox)).To(BeNil())
			sut.AddContainer(myContainer)
			Expect(sut.CtrIDIndex().Add(containerID)).To(BeNil())
			Expect(sut.PodIDIndex().Add(sandboxID)).To(BeNil())

			// When
			container, err := sut.GetContainerFromShortID(containerID)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(container).To(BeNil())
		})
	})
})
