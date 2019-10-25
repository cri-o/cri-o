package storage_test

import (
	"github.com/cri-o/cri-o/internal/pkg/storage"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = t.Describe("utils", func() {
	It("should be able to tell if a container belongs to CRI-O", func() {
		for _, testCase := range []struct {
			PodName string
			PodID   string
			IsCrio  bool
		}{
			{PodName: "", PodID: "", IsCrio: false},
		} {
			md := &storage.RuntimeContainerMetadata{
				PodName: testCase.PodName,
				PodID:   testCase.PodID,
			}
			Expect(storage.IsCrioContainer(md)).To(Equal(testCase.IsCrio))
		}
	})
})
