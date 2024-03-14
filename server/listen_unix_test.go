package server_test

import (
	"os"

	"github.com/cri-o/cri-o/server"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("Listen", func() {
	t.Describe("Listen", func() {
		It("should succeed", func() {
			// Given
			defer os.Remove("address")

			// When
			listener, err := server.Listen("unix", "address")

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(listener).NotTo(BeNil())
		})

		It("should fail when already bound", func() {
			// Given
			defer os.Remove("address")
			listener, err := server.Listen("unix", "address")
			Expect(err).ToNot(HaveOccurred())
			Expect(listener).NotTo(BeNil())

			// When
			listener, err = server.Listen("unix", "address")

			// Then
			Expect(err).To(HaveOccurred())
			Expect(listener).To(BeNil())
		})
	})
})
