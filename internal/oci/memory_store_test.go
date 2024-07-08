package oci_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/utils"
)

// The actual test suite.
var _ = t.Describe("MemoryStore", func() {
	t.Describe("NewMemoryStore", func() {
		It("should succeed to create a new memory store", func() {
			// Given
			// When
			store := oci.NewMemoryStore()

			// Then
			Expect(store).NotTo(BeNil())
		})
	})

	t.Describe("MemoryStore", func() {
		var (
			sut           oci.ContainerStorer
			testContainer *oci.Container
		)

		const containerID = "id"

		// Setup the test
		BeforeEach(func() {
			testContainer = getTestContainer()
			sut = oci.NewMemoryStore()
			Expect(sut).NotTo(BeNil())
		})

		It("should succeed to add a new container", func() {
			// Given
			// When
			sut.Add(containerID, testContainer)

			// Then
			Expect(sut.Get(containerID)).NotTo(BeNil())
			Expect(sut.Size()).To(BeEquivalentTo(1))
		})

		It("should succeed to delete a container", func() {
			// Given
			sut.Add(containerID, testContainer)
			Expect(sut.Get(containerID)).NotTo(BeNil())

			// When
			sut.Delete(containerID)

			// Then
			Expect(sut.Get(containerID)).To(BeNil())
			Expect(sut.Size()).To(BeZero())
		})

		It("should succeed to delete a non existing container", func() {
			// Given
			// When
			sut.Delete(containerID)

			// Then
			Expect(sut.Get(containerID)).To(BeNil())
			Expect(sut.Size()).To(BeZero())
		})

		It("should fail to get a non existing container", func() {
			// Given
			// When
			// Then
			Expect(sut.Get(containerID)).To(BeNil())
			Expect(sut.Size()).To(BeZero())
		})

		It("should succeed to list containers", func() {
			// Given
			sut.Add(containerID, testContainer)
			Expect(sut.Get(containerID)).NotTo(BeNil())

			// When
			containers := sut.List()

			// Then
			Expect(containers).NotTo(BeNil())
			Expect(len(containers)).To(BeEquivalentTo(1))
			Expect(containers[0]).To(Equal(testContainer))
		})

		It("should succeed to list containers in an empty store", func() {
			// Given
			// When
			containers := sut.List()

			// Then
			Expect(containers).NotTo(BeNil())
			Expect(containers).To(BeEmpty())
		})

		It("should succeed to get the first container with filter", func() {
			// Given
			sut.Add(containerID, testContainer)
			Expect(sut.Get(containerID)).NotTo(BeNil())

			// When
			container := sut.First(func(*oci.Container) bool { return true })

			// Then
			Expect(container).NotTo(BeNil())
			Expect(container).To(Equal(testContainer))
		})

		It("should succeed to get the first container without filter", func() {
			// Given
			sut.Add(containerID, testContainer)
			Expect(sut.Get(containerID)).NotTo(BeNil())

			// When
			container := sut.First(nil)

			// Then
			Expect(container).NotTo(BeNil())
			Expect(container).To(Equal(testContainer))
		})

		It("should fail to the first container with false filter", func() {
			// Given
			sut.Add(containerID, testContainer)
			Expect(sut.Get(containerID)).NotTo(BeNil())

			// When
			container := sut.First(func(*oci.Container) bool { return false })

			// Then
			Expect(container).To(BeNil())
		})

		It("should succeed apply", func() {
			// Given
			newContainerState := &oci.ContainerState{ExitCode: utils.Int32Ptr(-1)}
			sut.Add(containerID, testContainer)
			Expect(sut.Get(containerID)).NotTo(BeNil())

			// When
			sut.ApplyAll(func(container *oci.Container) {
				container.SetState(newContainerState)
			})

			// Then
			Expect(sut.Get(containerID).State()).To(Equal(newContainerState))
		})

		It("should succeed apply without valid function", func() {
			// Given
			sut.Add(containerID, testContainer)
			Expect(sut.Get(containerID)).NotTo(BeNil())

			// When
			sut.ApplyAll(nil)

			// Then
			Expect(sut.Get(containerID)).To(Equal(testContainer))
		})
	})
})
