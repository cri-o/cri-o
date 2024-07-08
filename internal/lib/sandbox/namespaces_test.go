package sandbox_test

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cri-o/cri-o/internal/config/nsmgr"
	nsmgrtest "github.com/cri-o/cri-o/internal/config/nsmgr/test"
)

const numNamespaces = 4

// The actual test suite.
var _ = t.Describe("SandboxManagedNamespaces", func() {
	// Setup the SUT
	BeforeEach(beforeEach)
	t.Describe("AddManagedNamespaces", func() {
		It("should succeed if nil", func() {
			// Given
			var managedNamespaces []nsmgr.Namespace

			// When
			testSandbox.AddManagedNamespaces(managedNamespaces)

			// Then
			Expect(testSandbox.NamespacePaths()).To(BeEmpty())
		})
		It("should succeed if empty", func() {
			// Given
			managedNamespaces := make([]nsmgr.Namespace, 0)

			// When
			testSandbox.AddManagedNamespaces(managedNamespaces)

			// Then
			Expect(testSandbox.NamespacePaths()).To(BeEmpty())
		})

		It("should succeed with valid namespaces", func() {
			// When
			testSandbox.AddManagedNamespaces(nsmgrtest.AllSpoofedNamespaces)

			// Then
			createdNamespaces := testSandbox.NamespacePaths()
			Expect(createdNamespaces).To(HaveLen(4))
		})
		It("should panic with invalid namespaces", func() {
			// Given
			// When
			ns := &nsmgrtest.SpoofedNamespace{
				NsType: "invalid",
			}
			// Then
			Expect(func() {
				testSandbox.AddManagedNamespaces([]nsmgr.Namespace{ns})
			}).To(Panic())
		})
	})
	t.Describe("RemoveManagedNamespaces", func() {
		It("should succeed when namespaces nil", func() {
			// Given
			// When
			err := testSandbox.RemoveManagedNamespaces()

			// Then
			Expect(err).ToNot(HaveOccurred())
		})
		It("should succeed when namespaces not nil", func() {
			// Given
			testSandbox.AddManagedNamespaces(nsmgrtest.AllSpoofedNamespaces)

			// When
			err := testSandbox.RemoveManagedNamespaces()

			// Then
			Expect(err).ToNot(HaveOccurred())
		})
	})
	t.Describe("*NsJoin", func() {
		It("should succeed when asked to join a network namespace", func() {
			// Given
			err := testSandbox.NetNsJoin("/proc/self/ns/net")

			// Then
			Expect(err).ToNot(HaveOccurred())
		})
		It("should succeed when asked to join a ipc namespace", func() {
			// Given
			err := testSandbox.IpcNsJoin("/proc/self/ns/ipc")

			// Then
			Expect(err).ToNot(HaveOccurred())
		})
		It("should succeed when asked to join a uts namespace", func() {
			// Given
			err := testSandbox.UtsNsJoin("/proc/self/ns/uts")

			// Then
			Expect(err).ToNot(HaveOccurred())
		})
		It("should succeed when asked to join a user namespace", func() {
			// Given
			err := testSandbox.UserNsJoin("/proc/self/ns/user")

			// Then
			Expect(err).ToNot(HaveOccurred())
		})
		It("should fail when network namespace not exists", func() {
			// Given
			// When
			err := testSandbox.NetNsJoin("path")

			// Then
			Expect(err).To(HaveOccurred())
		})
		It("should fail when uts namespace not exists", func() {
			// Given
			// When
			err := testSandbox.UtsNsJoin("path")

			// Then
			Expect(err).To(HaveOccurred())
		})
		It("should fail when ipc namespace not exists", func() {
			// Given
			// When
			err := testSandbox.IpcNsJoin("path")

			// Then
			Expect(err).To(HaveOccurred())
		})
		It("should fail when user namespace not exists", func() {
			// Given
			// When
			err := testSandbox.UserNsJoin("path")

			// Then
			Expect(err).To(HaveOccurred())
		})
		It("should fail when sandbox already has network namespace", func() {
			// Given
			testSandbox.AddManagedNamespaces(nsmgrtest.AllSpoofedNamespaces)

			// When
			err := testSandbox.NetNsJoin("/proc/self/ns/net")

			// Then
			Expect(err).To(HaveOccurred())
		})
		It("should fail when sandbox already has ipc namespace", func() {
			// Given
			testSandbox.AddManagedNamespaces(nsmgrtest.AllSpoofedNamespaces)

			// When
			err := testSandbox.IpcNsJoin("/proc/self/ns/ipc")

			// Then
			Expect(err).To(HaveOccurred())
		})
		It("should fail when sandbox already has uts namespace", func() {
			// Given
			testSandbox.AddManagedNamespaces(nsmgrtest.AllSpoofedNamespaces)

			// When
			err := testSandbox.UtsNsJoin("/proc/self/ns/uts")

			// Then
			Expect(err).To(HaveOccurred())
		})
		It("should fail when sandbox already has user namespace", func() {
			// Given
			testSandbox.AddManagedNamespaces(nsmgrtest.AllSpoofedNamespaces)

			// When
			err := testSandbox.UserNsJoin("/proc/self/ns/user")

			// Then
			Expect(err).To(HaveOccurred())
		})
		It("should fail when asked to join a non-namespace", func() {
			// Given
			// When
			err := testSandbox.NetNsJoin("/tmp")

			// Then
			Expect(err).To(HaveOccurred())
		})
		It("should fail when asked to join a non-namespace", func() {
			// Given

			// When
			err := testSandbox.IpcNsJoin("/tmp")

			// Then
			Expect(err).To(HaveOccurred())
		})
		It("should fail when asked to join a non-namespace", func() {
			// Given
			// When
			err := testSandbox.UtsNsJoin("/tmp")

			// Then
			Expect(err).To(HaveOccurred())
		})
		It("should fail when asked to join a non-namespace", func() {
			// Given
			// When
			err := testSandbox.UserNsJoin("/tmp")

			// Then
			Expect(err).To(HaveOccurred())
		})
	})
	t.Describe("*NsPath", func() {
		It("should get nothing when network not set", func() {
			// Given
			// When
			ns := testSandbox.NetNsPath()
			// Then
			Expect(ns).To(Equal(""))
		})
		It("should get nothing when ipc not set", func() {
			// Given
			// When
			ns := testSandbox.IpcNsPath()
			// Then
			Expect(ns).To(Equal(""))
		})
		It("should get nothing when uts not set", func() {
			// Given
			// When
			ns := testSandbox.UtsNsPath()
			// Then
			Expect(ns).To(Equal(""))
		})
		It("should get nothing when uts not set", func() {
			// Given
			// When
			ns := testSandbox.UserNsPath()
			// Then
			Expect(ns).To(Equal(""))
		})
		It("should get nothing when pid not set", func() {
			// Given
			// When
			ns := testSandbox.PidNsPath()
			// Then
			Expect(ns).To(Equal(""))
		})
		It("should get something when network is set", func() {
			// Given
			testSandbox.AddManagedNamespaces(nsmgrtest.AllSpoofedNamespaces)
			// When
			path := testSandbox.NetNsPath()
			// Then
			Expect(path).ToNot(Equal(""))
		})
		It("should get something when ipc is set", func() {
			// Given
			testSandbox.AddManagedNamespaces(nsmgrtest.AllSpoofedNamespaces)
			// When
			path := testSandbox.IpcNsPath()
			// Then
			Expect(path).ToNot(Equal(""))
		})
		It("should get something when uts is set", func() {
			// Given
			testSandbox.AddManagedNamespaces(nsmgrtest.AllSpoofedNamespaces)
			// When
			path := testSandbox.UtsNsPath()
			// Then
			Expect(path).ToNot(Equal(""))
		})
		It("should get something when user is set", func() {
			// Given
			testSandbox.AddManagedNamespaces(nsmgrtest.AllSpoofedNamespaces)
			// When
			path := testSandbox.UserNsPath()
			// Then
			Expect(path).ToNot(Equal(""))
		})
	})
	t.Describe("NamespacePaths with infra", func() {
		It("should get nothing when infra set but pid 0", func() {
			// Given
			infra, err := nsmgrtest.ContainerWithPid(0)
			Expect(err).ToNot(HaveOccurred())
			Expect(testSandbox.SetInfraContainer(infra)).To(Succeed())
			// When
			nsPaths := testSandbox.NamespacePaths()
			// Then
			Expect(nsPaths).To(BeEmpty())
			Expect(testSandbox.PidNsPath()).To(BeEmpty())
		})
		It("should get something when infra set and pid running", func() {
			// Given
			infra, err := nsmgrtest.ContainerWithPid(os.Getpid())
			Expect(err).ToNot(HaveOccurred())
			Expect(testSandbox.SetInfraContainer(infra)).To(Succeed())
			// When
			nsPaths := testSandbox.NamespacePaths()
			// Then
			for _, ns := range nsPaths {
				Expect(ns.Path()).To(ContainSubstring("/proc"))
			}
			Expect(nsPaths).To(HaveLen(numNamespaces))
			Expect(testSandbox.PidNsPath()).To(ContainSubstring("/proc"))
		})
		It("should get nothing when infra set with pid not running", func() {
			// Given
			// max valid pid is 4194304
			infra, err := nsmgrtest.ContainerWithPid(4194305)
			Expect(err).ToNot(HaveOccurred())
			Expect(testSandbox.SetInfraContainer(infra)).To(Succeed())
			// When
			nsPaths := testSandbox.NamespacePaths()
			// Then
			Expect(nsPaths).To(BeEmpty())
			Expect(testSandbox.PidNsPath()).To(BeEmpty())
		})
		It("should get managed path (except pid) despite infra set", func() {
			// Given
			infra, err := nsmgrtest.ContainerWithPid(os.Getpid())
			Expect(err).ToNot(HaveOccurred())
			Expect(testSandbox.SetInfraContainer(infra)).To(Succeed())
			// When
			testSandbox.AddManagedNamespaces(nsmgrtest.AllSpoofedNamespaces)
			nsPaths := testSandbox.NamespacePaths()
			// Then
			for _, ns := range nsPaths {
				Expect(ns.Path()).NotTo(ContainSubstring("/proc"))
			}
			Expect(nsPaths).To(HaveLen(numNamespaces))

			Expect(testSandbox.PidNsPath()).To(ContainSubstring("/proc"))
		})
	})
	t.Describe("NamespacePaths without infra", func() {
		It("should get nothing", func() {
			// Given
			// When
			nsPaths := testSandbox.NamespacePaths()
			// Then
			Expect(nsPaths).To(BeEmpty())
		})
	})
})
