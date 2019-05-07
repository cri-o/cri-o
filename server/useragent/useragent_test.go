package useragent_test

import (
	"context"

	"github.com/cri-o/cri-o/server/useragent"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("Useragent", func() {

	t.Describe("Get", func() {
		It("should succeed", func() {
			// Given
			// When
			result := useragent.Get(context.Background())

			// Then
			Expect(result).To(SatisfyAll(
				ContainSubstring("cri-o"),
				ContainSubstring("os"),
				ContainSubstring("arch"),
			))
		})
	})
})
