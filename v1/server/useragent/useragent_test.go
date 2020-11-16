package useragent_test

import (
	"github.com/cri-o/cri-o/v1/server/useragent"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("Useragent", func() {
	t.Describe("Get", func() {
		It("should succeed", func() {
			// Given
			// When
			result := useragent.Get()

			// Then
			Expect(result).To(SatisfyAll(
				ContainSubstring("cri-o"),
				ContainSubstring("os"),
				ContainSubstring("arch"),
			))
		})
	})
})
