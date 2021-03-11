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

	t.Describe("ContainerPause", func() {
		It("should fail with invalid container ID", func() {
			// Given

			// When
			res, err := sut.ContainerPause(context.Background(), "")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(Equal(""))
		})
	})

	t.Describe("ContainerUnpause", func() {
		It("should fail on invalid container", func() {
			// Given
			// When
			res, err := sut.ContainerUnpause(context.Background(), "")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeEmpty())
		})
	})
})
