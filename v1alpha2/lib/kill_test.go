package lib_test

import (
	"syscall"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("ContainerServer", func() {
	// Prepare the sut
	BeforeEach(beforeEach)

	t.Describe("ContainerKill", func() {
		It("should fail when not found", func() {
			// Given
			// When
			res, err := sut.ContainerKill("", syscall.SIGINT)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(Equal(""))
		})
	})
})
