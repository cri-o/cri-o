package config_test

import (
	"bytes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("Config", func() {
	BeforeEach(beforeEach)

	t.Describe("WriteTemplate", func() {
		It("should succeed", func() {
			// Given
			var wr bytes.Buffer

			// When
			err := sut.WriteTemplate(&wr)

			// Then
			Expect(err).To(BeNil())
		})
	})
})
