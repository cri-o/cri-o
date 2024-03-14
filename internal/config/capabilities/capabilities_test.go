package capabilities_test

import (
	"github.com/cri-o/cri-o/internal/config/capabilities"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("Capabilities", func() {
	It("should succeed to validate the default capabilities", func() {
		// Given
		sut := capabilities.Default()

		// When
		err := sut.Validate()

		// Then
		Expect(err).ToNot(HaveOccurred())
	})

	It("should succeed to validate wrong case capabilities", func() {
		// Given
		sut := capabilities.Capabilities{"chOwn", "setGID", "NET_raw"}

		// When
		err := sut.Validate()

		// Then
		Expect(err).ToNot(HaveOccurred())
	})

	It("should fail to validate wrong capabilities", func() {
		// Given
		sut := capabilities.Capabilities{"wrong"}

		// When
		err := sut.Validate()

		// Then
		Expect(err).To(HaveOccurred())
	})
})
