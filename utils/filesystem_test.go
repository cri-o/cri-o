package utils_test

import (
	"os"

	"github.com/cri-o/cri-o/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("Filesystem", func() {
	t.Describe("GetDiskUsageStats", func() {
		It("should succeed at the current working directory", func() {
			// Given
			// When
			bytes, inodes, err := utils.GetDiskUsageStats(".")

			// Then
			Expect(err).To(BeNil())
			Expect(bytes).To(SatisfyAll(BeNumerically(">", 0)))
			Expect(inodes).To(SatisfyAll(BeNumerically(">", 0)))
		})

		It("should fail on invalid path", func() {
			// Given
			// When
			bytes, inodes, err := utils.GetDiskUsageStats("/not-existing")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(bytes).To(BeEquivalentTo(0))
			Expect(inodes).To(BeEquivalentTo(0))
		})
	})

	t.Describe("IsDirectory", func() {
		It("should succeed on a directory", func() {
			Expect(utils.IsDirectory(".")).To(BeNil())
		})

		It("should fail on a file", func() {
			Expect(utils.IsDirectory(os.Args[0])).NotTo(BeNil())
		})

		It("should fail on a missing path", func() {
			Expect(utils.IsDirectory("/no/such/path")).NotTo(BeNil())
		})
	})
})
