package hostport

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = t.Describe("NoopHostportManager", func() {
	It("should succeed", func() {
		manager := NewNoopHostportManager()
		Expect(manager).NotTo(BeNil())

		err := manager.Add("id", nil)
		Expect(err).NotTo(HaveOccurred())

		err = manager.Remove("id", nil)
		Expect(err).NotTo(HaveOccurred())
	})
})
