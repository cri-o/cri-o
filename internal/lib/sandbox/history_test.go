package sandbox_test

import (
	"time"

	"github.com/cri-o/cri-o/internal/hostport"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/server/cri/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("History", func() {
	var sut *sandbox.History

	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		otherTestSandbox, err := sandbox.New("sandboxID", "", "", "", "",
			make(map[string]string), make(map[string]string), "", "",
			&types.PodSandboxMetadata{}, "", "", false, "", "", "",
			[]*hostport.PortMapping{}, false, time.Now(), "")
		Expect(err).To(BeNil())
		Expect(testSandbox).NotTo(BeNil())
		sut = &sandbox.History{testSandbox, otherTestSandbox}
	})

	t.Describe("Len", func() {
		It("should succeed", func() {
			// Given
			// When
			// Then
			Expect(sut.Len()).To(BeEquivalentTo(2))
		})
	})

	t.Describe("Less", func() {
		It("should succeed", func() {
			// Given
			// When
			// Then
			Expect(sut.Less(0, 0)).To(BeFalse())
			Expect(sut.Less(0, 1)).To(BeFalse())
			Expect(sut.Less(1, 0)).To(BeTrue())
		})
	})

	t.Describe("Swap", func() {
		It("should succeed", func() {
			// Given
			// When
			sut.Swap(0, 1)

			// Then
			Expect(sut.Less(0, 0)).To(BeFalse())
			Expect(sut.Less(0, 1)).To(BeTrue())
			Expect(sut.Less(1, 0)).To(BeFalse())
		})
	})
})
