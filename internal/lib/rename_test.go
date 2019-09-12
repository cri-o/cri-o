package lib_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("ContainerServer", func() {
	// Prepare the sut
	BeforeEach(beforeEach)

	t.Describe("ContainerRename", func() {
		It("should fail on invalid container ID", func() {
			// Given
			// When
			err := sut.ContainerRename("", "")

			// Then
			Expect(err).NotTo(BeNil())
		})
	})
})
