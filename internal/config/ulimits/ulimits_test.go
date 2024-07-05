package ulimits_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cri-o/cri-o/internal/config/ulimits"
)

var _ = t.Describe("New", func() {
	var sut *ulimits.Config

	It("should be empty without load", func() {
		// Given
		sut = ulimits.New()
		Expect(sut).NotTo(BeNil())

		// When
		res := sut.Ulimits()

		// Then
		Expect(res).To(BeEmpty())
	})
})

var _ = t.Describe("LoadUlimits", func() {
	var sut *ulimits.Config

	It("should fail if invalid", func() {
		// Given
		sut = ulimits.New()
		Expect(sut).NotTo(BeNil())

		// When
		err := sut.LoadUlimits([]string{"hi=-1:-1"})

		// Then
		Expect(sut.Ulimits()).To(BeEmpty())
		Expect(err).To(HaveOccurred())
	})
	It("should succeed if valid", func() {
		// Given
		sut = ulimits.New()
		Expect(sut).NotTo(BeNil())

		// When
		err := sut.LoadUlimits([]string{"locks=10:64"})

		// Then
		Expect(err).ToNot(HaveOccurred())
		Expect(sut.Ulimits()).NotTo(BeEmpty())
	})
})
