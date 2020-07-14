package server

import (
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("Secrets", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})
	AfterEach(afterEach)

	t.Describe("SaveTo", func() {
		It("should succeed", func() {
			// Given
			sut := SecretData{}
			secretsDir := "secrets"
			defer os.RemoveAll(secretsDir)

			// When
			err := sut.SaveTo(secretsDir)

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail with invalid dir", func() {
			// Given
			sut := SecretData{}
			secretsDir := "/proc/invalid" // nolint: gosec

			// When
			err := sut.SaveTo(secretsDir)

			// Then
			Expect(err).NotTo(BeNil())
		})
	})
})
