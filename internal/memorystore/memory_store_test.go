package memorystore_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/memorystore"
)

// The actual test suite.
var _ = t.Describe("MemoryStore", func() {
	var sut memorystore.Storer[*sandbox.Sandbox]

	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		sut = memorystore.New[*sandbox.Sandbox]()
		Expect(sut).NotTo(BeNil())
	})

	t.Describe("Add", func() {
		It("should succeed", func() {
			// Given
			const sandboxID = "id"

			// When
			sut.Add(sandboxID, testSandbox)

			// Then
			Expect(sut.Get(sandboxID)).NotTo(BeNil())
			Expect(sut.Get("otherSandbox")).To(BeNil())
		})
	})

	t.Describe("Delete", func() {
		It("should succeed", func() {
			// Given
			const sandboxID = "id"
			sut.Add(sandboxID, testSandbox)
			Expect(sut.Get(sandboxID)).NotTo(BeNil())

			// When
			sut.Delete(sandboxID)

			// Then
			Expect(sut.Get(sandboxID)).To(BeNil())
		})
	})

	t.Describe("List", func() {
		It("should succeed", func() {
			// Given
			const sandboxID = "id"
			sut.Add(sandboxID, testSandbox)
			Expect(sut.Get(sandboxID)).NotTo(BeNil())

			// When
			sandboxes := sut.List()

			// Then
			Expect(sandboxes).NotTo(BeNil())
			Expect(sandboxes).To(HaveLen(sut.Size()))
			Expect(len(sandboxes)).To(BeEquivalentTo(1))
		})
	})

	t.Describe("First", func() {
		It("should not be nil on filtered", func() {
			// Given
			const sandboxID = "id"
			sut.Add(sandboxID, testSandbox)
			Expect(sut.Get(sandboxID)).NotTo(BeNil())

			// When
			first := sut.First(func(*sandbox.Sandbox) bool { return true })

			// Then
			Expect(first).NotTo(BeNil())
			Expect(first).To(Equal(testSandbox))
		})

		It("should be nil on not filtered", func() {
			// Given
			const sandboxID = "id"
			sut.Add(sandboxID, testSandbox)
			Expect(sut.Get(sandboxID)).NotTo(BeNil())

			// When
			first := sut.First(func(*sandbox.Sandbox) bool { return false })

			// Then
			Expect(first).To(BeNil())
		})
	})

	t.Describe("ApplyAll", func() {
		It("should succeed", func() {
			// Given
			const sandboxID = "id"
			sut.Add(sandboxID, testSandbox)
			Expect(sut.Get(sandboxID)).NotTo(BeNil())

			// When
			called := 0
			sut.ApplyAll(func(*sandbox.Sandbox) {
				called++
			})

			// Then
			Expect(called).To(Equal(1))
		})
	})
})
