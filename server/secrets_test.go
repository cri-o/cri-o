package server_test

import (
	"os"

	"github.com/cri-o/cri-o/server"
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
			sut := server.SecretData{}
			secretsDir := "secrets"
			defer os.RemoveAll(secretsDir)

			// When
			err := sut.SaveTo(secretsDir)

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail with invalid dir", func() {
			// Given
			sut := server.SecretData{}
			secretsDir := "/proc/invalid" // nolint: gosec

			// When
			err := sut.SaveTo(secretsDir)

			// Then
			Expect(err).NotTo(BeNil())
		})
	})
})
