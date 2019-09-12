package lib_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("ContainerServer", func() {
	// Prepare the sut
	BeforeEach(beforeEach)

	t.Describe("ContainerWait", func() {
		It("should fail on invalid container ID", func() {
			// Given
			// When
			res, err := sut.ContainerWait("")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeEquivalentTo(0))
		})
	})
})
