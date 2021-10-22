package nri_test

import (
	"os"

	"github.com/cri-o/cri-o/internal/config/nri"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func tempFileWithData(data string) string {
	f := t.MustTempFile("")
	Expect(os.WriteFile(f, []byte(data), 0o644)).To(BeNil())
	return f
}

// NRI configuration tests.
var _ = t.Describe("When parsing NRI config file", func() {
	t.Describe("non-existent file", func() {
		It("should return an error", func() {
			cfg := nri.New()
			cfg.Enabled = true
			cfg.ConfigPath = "non-existent-file"
			err := cfg.Validate(true)
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("invalid file format", func() {
		It("should return an error", func() {
			f := tempFileWithData(`fooBar:
  - none
`)
			cfg := nri.New()
			cfg.Enabled = true
			cfg.ConfigPath = f
			err := cfg.Validate(true)
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("correct file format", func() {
		It("should not return an error", func() {
			f := tempFileWithData(`disableConnections: true
`)
			cfg := nri.New()
			cfg.Enabled = true
			cfg.ConfigPath = f
			err := cfg.Validate(true)
			Expect(err).To(BeNil())
		})
	})
})
