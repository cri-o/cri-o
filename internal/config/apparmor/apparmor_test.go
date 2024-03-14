package apparmor_test

import (
	"github.com/cri-o/cri-o/internal/config/apparmor"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("Config", func() {
	var sut *apparmor.Config

	BeforeEach(func() {
		sut = apparmor.New()
		Expect(sut).NotTo(BeNil())

		if !sut.IsEnabled() {
			Skip("AppArmor is disabled")
		}
	})

	t.Describe("IsEnabled", func() {
		It("should be true per default", func() {
			// Given
			// When
			res := sut.IsEnabled()

			// Then
			Expect(res).To(BeTrue())
		})
	})

	t.Describe("LoadProfile", func() {
		It("should succeed with unconfined", func() {
			// Given
			// When
			err := sut.LoadProfile("unconfined")

			// Then
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
