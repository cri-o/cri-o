package statsmgr_test

import (
	"github.com/cri-o/cri-o/internal/config/statsmgr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = t.Describe("StatsManager", func() {
	t.Describe("GetDiskUsageStats", func() {
		It("should succeed at the current working directory", func() {
			// Given
			// When
			bytes, inodes, err := statsmgr.GetDiskUsageStats(".")

			// Then
			Expect(err).To(BeNil())
			Expect(bytes).To(SatisfyAll(BeNumerically(">", 0)))
			Expect(inodes).To(SatisfyAll(BeNumerically(">", 0)))
		})

		It("should fail on invalid path", func() {
			// Given
			// When
			bytes, inodes, err := statsmgr.GetDiskUsageStats("/not-existing")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(bytes).To(BeEquivalentTo(0))
			Expect(inodes).To(BeEquivalentTo(0))
		})
	})
})
