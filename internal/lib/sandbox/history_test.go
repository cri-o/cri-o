package sandbox_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/hostport"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
)

// The actual test suite.
var _ = t.Describe("History", func() {
	var sut *sandbox.History

	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		sbuilder := sandbox.NewBuilder()
		sbuilder.SetID("sandboxID")
		sbuilder.SetName("")
		sbuilder.SetNamespace("")
		sbuilder.SetKubeName("")
		sbuilder.SetLogDir("test")
		sbuilder.SetCriSandbox(sbuilder.ID(), time.Now(), make(map[string]string), make(map[string]string), &types.PodSandboxMetadata{})
		sbuilder.SetShmPath("")
		sbuilder.SetCgroupParent("")
		sbuilder.SetPrivileged(false)
		sbuilder.SetRuntimeHandler("")
		sbuilder.SetResolvPath("")
		sbuilder.SetHostname("")
		sbuilder.SetPortMappings([]*hostport.PortMapping{})
		sbuilder.SetHostNetwork(false)
		sbuilder.SetUsernsMode("")
		sbuilder.SetPodLinuxOverhead(nil)
		sbuilder.SetPodLinuxResources(nil)

		otherTestSandbox := sbuilder.GetSandbox()

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
