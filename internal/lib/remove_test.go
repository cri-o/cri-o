package lib_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("ContainerServer", func() {
	// Prepare the sut
	BeforeEach(beforeEach)

	t.Describe("Remove", func() {
		It("should fail on invalid container ID", func() {
			// Given
			// When
			res, err := sut.Remove(context.Background(), "", "", false)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeEmpty())
		})
	})
})
