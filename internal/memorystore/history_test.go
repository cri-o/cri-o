package memorystore_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/hostport"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/memorystore"
)

// The actual test suite.
var _ = t.Describe("History", func() {
	var sut memorystore.History[*sandbox.Sandbox]

	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		createdAt := time.Now()
		sbox := sandbox.NewBuilder()
		sbox.SetID("sandboxID")
		sbox.SetName("sandboxName")
		sbox.SetLogDir("test")
		sbox.SetShmPath("test")
		sbox.SetNamespace("")
		sbox.SetKubeName("")
		sbox.SetMountLabel("test")
		sbox.SetProcessLabel("test")
		sbox.SetCgroupParent("")
		sbox.SetRuntimeHandler("")
		sbox.SetResolvPath("")
		sbox.SetHostname("")
		sbox.SetPortMappings([]*hostport.PortMapping{})
		sbox.SetHostNetwork(false)
		sbox.SetUsernsMode("")
		sbox.SetPodLinuxOverhead(nil)
		sbox.SetPodLinuxResources(nil)
		sbox.SetCreatedAt(createdAt)
		err := sbox.SetCRISandbox(sbox.ID(), make(map[string]string), make(map[string]string), &types.PodSandboxMetadata{})
		Expect(err).ToNot(HaveOccurred())
		sbox.SetPrivileged(false)
		sbox.SetPortMappings([]*hostport.PortMapping{})
		sbox.SetHostNetwork(false)
		sbox.SetCreatedAt(createdAt)

		otherTestSandbox, err := sbox.GetSandbox()
		Expect(err).ToNot(HaveOccurred())

		Expect(testSandbox).NotTo(BeNil())
		sut = memorystore.History[*sandbox.Sandbox]{testSandbox, otherTestSandbox}
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
