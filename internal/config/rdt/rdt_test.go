package rdt

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func tempFileWithData(data string) string {
	f := t.MustTempFile("")
	Expect(os.WriteFile(f, []byte(data), 0o644)).To(Succeed())
	return f
}

// The actual test suite.
var _ = t.Describe("When parsing RDT config file", func() {
	t.Describe("non-existent file", func() {
		It("should return an error", func() {
			_, err := loadConfigFile("non-existent-file")
			Expect(err).To(HaveOccurred())
		})
	})

	t.Describe("invalid file format", func() {
		It("should return an error", func() {
			f := tempFileWithData(`partitions:
- foo
`)
			_, err := loadConfigFile(f)
			Expect(err).To(HaveOccurred())
		})
	})

	t.Describe("correct file format", func() {
		It("should not return an error", func() {
			f := tempFileWithData(`partitions:
  default:
    l3Allocation: 100%
    classes:
      default:
`)
			_, err := loadConfigFile(f)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
