package blockio_test

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cri-o/cri-o/internal/config/blockio"
)

func tempFileWithData(data string) string {
	f := t.MustTempFile("")
	Expect(os.WriteFile(f, []byte(data), 0o644)).To(Succeed())
	return f
}

var _ = t.Describe("New", func() {
	var sut *blockio.Config

	It("should be disabled before load", func() {
		// Given
		sut = blockio.New()
		Expect(sut).NotTo(BeNil())

		// When
		res := sut.Enabled()

		// Then
		Expect(res).To(BeFalse())
	})
})

// The actual test suite.
var _ = t.Describe("Load", func() {
	t.Describe("non-existent file", func() {
		It("should return an error and disable blockio", func() {
			// Given
			sut := blockio.New()
			Expect(sut).NotTo(BeNil())

			// When
			err := sut.Load("non-existent-file")

			// Then
			Expect(err).To(HaveOccurred())
			Expect(sut.Enabled()).To(BeFalse())
		})
	})

	t.Describe("invalid file format", func() {
		It("should return an error and disable blockio", func() {
			// Given
			sut := blockio.New()
			Expect(sut).NotTo(BeNil())
			f := tempFileWithData(`classes:
- Weight: 10
`)
			// When
			err := sut.Load(f)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(sut.Enabled()).To(BeFalse())
		})
	})

	t.Describe("correct file format", func() {
		It("should enable blockio without errors", func() {
			// Given
			sut := blockio.New()
			Expect(sut).NotTo(BeNil())
			Expect(sut.Enabled()).NotTo(BeTrue())
			f := tempFileWithData(`classes:
  lowprio:
  - Weight: 20
  highprio:
  - Weight: 800
`)
			// When
			err := sut.Load(f)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(sut.Enabled()).To(BeTrue())
		})
	})
})
