package server

import (
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("Listen", func() {
	t.Describe("Listen", func() {
		It("should succeed", func() {
			// Given
			defer os.Remove("address")

			// When
			listener, err := Listen("unix", "address")

			// Then
			Expect(err).To(BeNil())
			Expect(listener).NotTo(BeNil())
		})

		It("should fail when already bound", func() {
			// Given
			defer os.Remove("address")
			listener, err := Listen("unix", "address")
			Expect(err).To(BeNil())
			Expect(listener).NotTo(BeNil())

			// When
			listener, err = Listen("unix", "address")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(listener).To(BeNil())
		})
	})
})
