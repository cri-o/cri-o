package sandbox_test

import (
	"os"
	"path/filepath"
	"time"

	"github.com/cri-o/cri-o/internal/config/nsmgr"
	"github.com/cri-o/cri-o/internal/oci"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

const numNamespaces = 4

type spoofedIface struct {
	nsType  nsmgr.NSType
	removed bool
}

func (s *spoofedIface) Type() nsmgr.NSType {
	return s.nsType
}

func (s *spoofedIface) Close() error {
	return nil
}

func (s *spoofedIface) Remove() error {
	s.removed = true
	return nil
}

func (s *spoofedIface) Path() string {
	return filepath.Join("tmp", string(s.nsType))
}

var allManagedNamespaces = []nsmgr.Namespace{
	&spoofedIface{
		nsType: nsmgr.IPCNS,
	},
	&spoofedIface{
		nsType: nsmgr.UTSNS,
	},
	&spoofedIface{
		nsType: nsmgr.NETNS,
	},
	&spoofedIface{
		nsType: nsmgr.USERNS,
	},
}

// The actual test suite
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
			Expect(len(testSandbox.NamespacePaths())).To(Equal(0))
		})
		It("should succeed if empty", func() {
			// Given
			managedNamespaces := make([]nsmgr.Namespace, 0)

			// When
			testSandbox.AddManagedNamespaces(managedNamespaces)

			// Then
			Expect(len(testSandbox.NamespacePaths())).To(Equal(0))
		})

		It("should succeed with valid namespaces", func() {
			// When
			testSandbox.AddManagedNamespaces(allManagedNamespaces)

			// Then
			createdNamespaces := testSandbox.NamespacePaths()
			Expect(len(createdNamespaces)).To(Equal(4))
		})
		It("should panic with invalid namespaces", func() {
			// Given
			// When
			ns := &spoofedIface{
				nsType: "invalid",
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
			Expect(err).To(BeNil())
		})
		It("should succeed when namespaces not nil", func() {
			// Given
			testSandbox.AddManagedNamespaces(allManagedNamespaces)

			// When
			err := testSandbox.RemoveManagedNamespaces()

			// Then
			Expect(err).To(BeNil())
		})
	})
	t.Describe("*NsJoin", func() {
		It("should succeed when asked to join a network namespace", func() {
			// Given
			err := testSandbox.NetNsJoin("/proc/self/ns/net")

			// Then
			Expect(err).To(BeNil())
		})
		It("should succeed when asked to join a ipc namespace", func() {
			// Given
			err := testSandbox.IpcNsJoin("/proc/self/ns/ipc")

			// Then
			Expect(err).To(BeNil())
		})
		It("should succeed when asked to join a uts namespace", func() {
			// Given
			err := testSandbox.UtsNsJoin("/proc/self/ns/uts")

			// Then
			Expect(err).To(BeNil())
		})
		It("should succeed when asked to join a user namespace", func() {
			// Given
			err := testSandbox.UserNsJoin("/proc/self/ns/user")

			// Then
			Expect(err).To(BeNil())
		})
		It("should fail when network namespace not exists", func() {
			// Given
			// When
			err := testSandbox.NetNsJoin("path")

			// Then
			Expect(err).NotTo(BeNil())
		})
		It("should fail when uts namespace not exists", func() {
			// Given
			// When
			err := testSandbox.UtsNsJoin("path")

			// Then
			Expect(err).NotTo(BeNil())
		})
		It("should fail when ipc namespace not exists", func() {
			// Given
			// When
			err := testSandbox.IpcNsJoin("path")

			// Then
			Expect(err).NotTo(BeNil())
		})
		It("should fail when user namespace not exists", func() {
			// Given
			// When
			err := testSandbox.UserNsJoin("path")

			// Then
			Expect(err).NotTo(BeNil())
		})
		It("should fail when sandbox already has network namespace", func() {
			// Given
			testSandbox.AddManagedNamespaces(allManagedNamespaces)

			// When
			err := testSandbox.NetNsJoin("/proc/self/ns/net")

			// Then
			Expect(err).NotTo(BeNil())
		})
		It("should fail when sandbox already has ipc namespace", func() {
			// Given
			testSandbox.AddManagedNamespaces(allManagedNamespaces)

			// When
			err := testSandbox.IpcNsJoin("/proc/self/ns/ipc")

			// Then
			Expect(err).NotTo(BeNil())
		})
		It("should fail when sandbox already has uts namespace", func() {
			// Given
			testSandbox.AddManagedNamespaces(allManagedNamespaces)

			// When
			err := testSandbox.UtsNsJoin("/proc/self/ns/uts")

			// Then
			Expect(err).NotTo(BeNil())
		})
		It("should fail when sandbox already has user namespace", func() {
			// Given
			testSandbox.AddManagedNamespaces(allManagedNamespaces)

			// When
			err := testSandbox.UserNsJoin("/proc/self/ns/user")

			// Then
			Expect(err).NotTo(BeNil())
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
			testSandbox.AddManagedNamespaces(allManagedNamespaces)
			// When
			path := testSandbox.NetNsPath()
			// Then
			Expect(path).ToNot(Equal(""))
		})
		It("should get something when ipc is set", func() {
			// Given
			testSandbox.AddManagedNamespaces(allManagedNamespaces)
			// When
			path := testSandbox.IpcNsPath()
			// Then
			Expect(path).ToNot(Equal(""))
		})
		It("should get something when uts is set", func() {
			// Given
			testSandbox.AddManagedNamespaces(allManagedNamespaces)
			// When
			path := testSandbox.UtsNsPath()
			// Then
			Expect(path).ToNot(Equal(""))
		})
		It("should get something when user is set", func() {
			// Given
			testSandbox.AddManagedNamespaces(allManagedNamespaces)
			// When
			path := testSandbox.UserNsPath()
			// Then
			Expect(path).ToNot(Equal(""))
		})
	})
	t.Describe("NamespacePaths with infra", func() {
		setupInfraContainerWithPid := func(pid int) {
			testContainer, err := oci.NewContainer("testid", "testname", "",
				"/container/logs", map[string]string{},
				map[string]string{}, map[string]string{}, "image",
				"imageName", "imageRef", &oci.Metadata{},
				"testsandboxid", false, false, false, "",
				"/root/for/container", time.Now(), "SIGKILL")
			Expect(err).To(BeNil())
			Expect(testContainer).NotTo(BeNil())

			cstate := &oci.ContainerState{}
			cstate.State = specs.State{
				Pid: pid,
			}
			// eat error here because callers may send invalid pids to test against
			_ = cstate.SetInitPid(pid) // nolint:errcheck
			testContainer.SetState(cstate)

			Expect(testSandbox.SetInfraContainer(testContainer)).To(BeNil())
		}

		It("should get nothing when infra set but pid 0", func() {
			// Given
			setupInfraContainerWithPid(0)
			// When
			nsPaths := testSandbox.NamespacePaths()
			// Then
			Expect(len(nsPaths)).To(Equal(0))
			Expect(testSandbox.PidNsPath()).To(BeEmpty())
		})
		It("should get something when infra set and pid running", func() {
			// Given
			setupInfraContainerWithPid(os.Getpid())
			// When
			nsPaths := testSandbox.NamespacePaths()
			// Then
			for _, ns := range nsPaths {
				Expect(ns.Path()).To(ContainSubstring("/proc"))
			}
			Expect(len(nsPaths)).To(Equal(numNamespaces))
			Expect(testSandbox.PidNsPath()).To(ContainSubstring("/proc"))
		})
		It("should get nothing when infra set with pid not running", func() {
			// Given
			// max valid pid is 4194304
			setupInfraContainerWithPid(4194305)
			// When
			nsPaths := testSandbox.NamespacePaths()
			// Then
			Expect(len(nsPaths)).To(Equal(0))
			Expect(testSandbox.PidNsPath()).To(BeEmpty())
		})
		It("should get managed path (except pid) despite infra set", func() {
			// Given
			setupInfraContainerWithPid(os.Getpid())
			// When
			testSandbox.AddManagedNamespaces(allManagedNamespaces)
			nsPaths := testSandbox.NamespacePaths()
			// Then
			for _, ns := range nsPaths {
				Expect(ns.Path()).NotTo(ContainSubstring("/proc"))
			}
			Expect(len(nsPaths)).To(Equal(numNamespaces))

			Expect(testSandbox.PidNsPath()).To(ContainSubstring("/proc"))
		})
	})
	t.Describe("NamespacePaths without infra", func() {
		It("should get nothing", func() {
			// Given
			// When
			nsPaths := testSandbox.NamespacePaths()
			// Then
			Expect(len(nsPaths)).To(Equal(0))
		})
	})
})
