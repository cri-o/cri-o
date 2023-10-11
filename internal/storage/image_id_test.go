package storage_test

import (
	"fmt"

	"github.com/cri-o/cri-o/internal/storage"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const testSHA256 = "2a03a6059f21e150ae84b0973863609494aad70f0a80eaeb64bddd8d92465812"

var _ = t.Describe("StorageImageID", func() {
	It("Should parse valid IDs", func() {
		id, err := storage.ParseStorageImageIDFromOutOfProcessData(testSHA256)
		Expect(err).To(BeNil())
		Expect(id.IDStringForOutOfProcessConsumptionOnly()).To(Equal(testSHA256))
	})

	It("Should reject invalid IDs", func() {
		for _, input := range []string{
			"",
			"@",
			testSHA256[:len(testSHA256)-1],
		} {
			_, err := storage.ParseStorageImageIDFromOutOfProcessData(input)
			Expect(err).NotTo(BeNil())
		}
	})

	It("Should reject use of uninitialized/empty values", func() {
		id := storage.StorageImageID{}
		Expect(func() { _ = id.IDStringForOutOfProcessConsumptionOnly() }).To(Panic())
	})

	It("Should be usable for logging, but not otherwise expose a string value", func() {
		id, err := storage.ParseStorageImageIDFromOutOfProcessData(testSHA256)
		Expect(err).To(BeNil())

		var _ fmt.Formatter = id // A compile-time check that id implements Formatter

		// We need an intermediate any() value, otherwise Go refuses to allow a compile-time-known check to be done at runtime.
		_, isStringer := any(id).(fmt.Stringer)
		Expect(isStringer).To(BeFalse())

		res := fmt.Sprintf("%s", id)
		Expect(res).To(Equal(testSHA256))
		res = fmt.Sprintf("%q", id)
		Expect(res).To(Equal(`"` + testSHA256 + `"`))
	})
})
