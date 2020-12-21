package utils_test

import (
	"os"

	"github.com/cri-o/cri-o/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("Filesystem", func() {
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
