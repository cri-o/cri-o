package useragent_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cri-o/cri-o/server/useragent"
)

// The actual test suite.
var _ = t.Describe("Useragent", func() {
	t.Describe("Get", func() {
		It("should succeed", func() {
			// Given
			// When
			result := useragent.AppendVersions("base",
				useragent.VersionInfo{"name", "0.1.0"},
				useragent.VersionInfo{"another", "0.2.0"},
			)

			// Then
			Expect(result).To(Equal("base name/0.1.0 another/0.2.0"))
		})

		It("should succeed with empty string", func() {
			// Given
			// When
			result := useragent.AppendVersions("")

			// Then
			Expect(result).To(BeEmpty())
		})

		It("should skip invalid name in VersionInfo", func() {
			// Given
			// When
			result := useragent.AppendVersions("",
				useragent.VersionInfo{Name: "\n", Version: "0.1.0"})

			// Then
			Expect(result).To(BeEmpty())
		})

		It("should skip invalid version in VersionInfo", func() {
			// Given
			// When
			result := useragent.AppendVersions("",
				useragent.VersionInfo{Name: "name", Version: "\n"})

			// Then
			Expect(result).To(BeEmpty())
		})
	})
})
