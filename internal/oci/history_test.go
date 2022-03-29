package oci_test

import (
	"github.com/cri-o/cri-o/internal/oci"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("History", func() {
	// The system under test
	var sut oci.History

	// Setup the test
	BeforeEach(func() {
		testContainer1 := getTestContainer()
		testContainer2 := getTestContainer()
		sut = oci.History([]*oci.Container{testContainer1, testContainer2})
	})

	It("should succeed to get the history len", func() {
		// Given
		// When
		// Then
		Expect(sut.Len()).To(BeEquivalentTo(2))
	})

	It("should succeed compare the creation time", func() {
		// Given
		// When
		res := sut.Less(1, 0)

		// Then
		Expect(res).To(BeTrue())
	})

	It("should succeed to swap items", func() {
		// Given
		sut.Swap(0, 1)

		// When
		res := sut.Less(1, 0)

		// Then
		Expect(res).To(BeFalse())
	})
})
