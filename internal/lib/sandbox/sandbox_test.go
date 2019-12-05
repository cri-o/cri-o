package sandbox_test

import (
	"os"
	"time"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"k8s.io/kubernetes/pkg/kubelet/dockershim/network/hostport"
)

// The actual test suite
var _ = t.Describe("Sandbox", func() {
	// Setup the SUT
	BeforeEach(beforeEach)

	t.Describe("New", func() {
		It("should succeed", func() {
			// Given
			id := "id"
			namespace := "namespace"
			name := "name"
			kubeName := "kubeName"
			logDir := "logDir"
			labels := map[string]string{"a": "labelA", "b": "labelB"}
			annotations := map[string]string{"a": "annotA", "b": "annotB"}
			processLabel := "processLabel"
			mountLabel := "mountLabel"
			metadata := pb.PodSandboxMetadata{Name: name}
			shmPath := "shmPath"
			cgroupParent := "cgroupParent"
			privileged := true
			runtimeHandler := "runtimeHandler"
			resolvPath := "resolvPath"
			hostname := "hostname"
			portMappings := []*hostport.PortMapping{{}, {}}
			hostNetwork := false

			// When
			sandbox, err := sandbox.New(id, namespace, name, kubeName, logDir,
				labels, annotations, processLabel, mountLabel, &metadata,
				shmPath, cgroupParent, privileged, runtimeHandler,
				resolvPath, hostname, portMappings, hostNetwork)

			// Then
			Expect(err).To(BeNil())
			Expect(sandbox).NotTo(BeNil())
			Expect(sandbox.ID()).To(Equal(id))
			Expect(sandbox.Namespace()).To(Equal(namespace))
			Expect(sandbox.Name()).To(Equal(name))
			Expect(sandbox.KubeName()).To(Equal(kubeName))
			Expect(sandbox.LogDir()).To(Equal(logDir))
			Expect(sandbox.Labels()).To(ConsistOf([]string{"labelA", "labelB"}))
			Expect(sandbox.Annotations()).To(ConsistOf([]string{"annotA", "annotB"}))
			Expect(sandbox.ProcessLabel()).To(Equal(processLabel))
			Expect(sandbox.MountLabel()).To(Equal(mountLabel))
			Expect(sandbox.Metadata().Name).To(Equal(name))
			Expect(sandbox.ShmPath()).To(Equal(shmPath))
			Expect(sandbox.CgroupParent()).To(Equal(cgroupParent))
			Expect(sandbox.Privileged()).To(Equal(privileged))
			Expect(sandbox.RuntimeHandler()).To(Equal(runtimeHandler))
			Expect(sandbox.ResolvPath()).To(Equal(resolvPath))
			Expect(sandbox.Hostname()).To(Equal(hostname))
			Expect(sandbox.PortMappings()).To(Equal(portMappings))
			Expect(sandbox.HostNetwork()).To(Equal(hostNetwork))
			Expect(sandbox.StopMutex()).NotTo(BeNil())
			Expect(sandbox.Containers()).NotTo(BeNil())
			Expect(sandbox.NetNs()).To(BeNil())
		})
	})

	t.Describe("SetSeccompProfilePath", func() {
		It("should succeed", func() {
			// Given
			newPath := "/some/path"
			Expect(testSandbox.SeccompProfilePath()).NotTo(Equal(newPath))

			// When
			testSandbox.SetSeccompProfilePath(newPath)

			// Then
			Expect(testSandbox.SeccompProfilePath()).To(Equal(newPath))
		})
	})

	t.Describe("AddIPs", func() {
		It("should succeed", func() {
			// Given
			newIPs := []string{"10.0.0.1"}
			Expect(testSandbox.IPs()).NotTo(Equal(newIPs))

			// When
			testSandbox.AddIPs(newIPs)

			// Then
			Expect(testSandbox.IPs()).To(Equal(newIPs))
		})
	})

	t.Describe("Stopped", func() {
		It("should succeed", func() {
			// Given
			Expect(testSandbox.Stopped()).To(BeFalse())

			// When
			testSandbox.SetStopped()

			// Then
			Expect(testSandbox.Stopped()).To(BeTrue())
		})
	})

	t.Describe("Created", func() {
		It("should succeed", func() {
			// Given
			Expect(testSandbox.Created()).To(BeFalse())

			// When
			testSandbox.SetCreated()

			// Then
			Expect(testSandbox.Created()).To(BeTrue())
		})
	})

	t.Describe("AddHostnamePath", func() {
		It("should succeed", func() {
			// Given
			newHostnamePath := "hostnamePath"
			Expect(testSandbox.HostnamePath()).NotTo(Equal(newHostnamePath))

			// When
			testSandbox.AddHostnamePath(newHostnamePath)

			// Then
			Expect(testSandbox.HostnamePath()).To(Equal(newHostnamePath))
		})
	})

	t.Describe("SetNamespaceOptions", func() {
		It("should succeed", func() {
			// Given
			newNamespaceOption := &pb.NamespaceOption{
				Network: 1,
				Pid:     2,
				Ipc:     3,
			}
			Expect(testSandbox.NamespaceOptions()).NotTo(Equal(newNamespaceOption))

			// When
			testSandbox.SetNamespaceOptions(newNamespaceOption)

			// Then
			Expect(testSandbox.NamespaceOptions().Network).
				To(Equal(newNamespaceOption.Network))
			Expect(testSandbox.NamespaceOptions().Pid).
				To(Equal(newNamespaceOption.Pid))
			Expect(testSandbox.NamespaceOptions().Ipc).
				To(Equal(newNamespaceOption.Ipc))

		})
	})

	t.Describe("Container", func() {
		var testContainer *oci.Container

		BeforeEach(func() {
			var err error
			testContainer, err = oci.NewContainer("testid", "testname", "",
				"/container/logs", "", map[string]string{},
				map[string]string{}, map[string]string{}, "image",
				"imageName", "imageRef", &pb.ContainerMetadata{},
				"testsandboxid", false, false, false, false, "",
				"/root/for/container", time.Now(), "SIGKILL")
			Expect(err).To(BeNil())
			Expect(testContainer).NotTo(BeNil())

		})

		It("should succeed to add and remove a container", func() {
			// Given
			Expect(testSandbox.GetContainer(testContainer.Name())).To(BeNil())

			// When
			testSandbox.AddContainer(testContainer)

			// Then
			Expect(testSandbox.GetContainer(testContainer.Name())).
				To(Equal(testContainer))

			// And When
			testSandbox.RemoveContainer(testContainer)

			// Then
			Expect(testSandbox.GetContainer(testContainer.Name())).To(BeNil())
		})

		It("should succeed to add and remove an infra container", func() {
			// Given
			Expect(testSandbox.InfraContainer()).To(BeNil())

			// When
			err := testSandbox.SetInfraContainer(testContainer)

			// Then
			Expect(err).To(BeNil())
			Expect(testSandbox.InfraContainer()).To(Equal(testContainer))
			Expect(testSandbox.UserNsPath()).NotTo(Equal(""))
			Expect(testSandbox.NetNsPath()).NotTo(Equal(""))

			// And When
			testSandbox.RemoveInfraContainer()

			// Then
			Expect(testSandbox.InfraContainer()).To(BeNil())
			Expect(testSandbox.UserNsPath()).To(Equal(""))
			Expect(testSandbox.NetNsPath()).To(Equal(""))
		})

		It("should fail add an infra container twice", func() {
			// Given
			Expect(testSandbox.InfraContainer()).To(BeNil())
			Expect(testSandbox.SetInfraContainer(testContainer)).To(BeNil())

			// When
			err := testSandbox.SetInfraContainer(testContainer)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail to set a nil infra container", func() {
			// Given
			// When
			err := testSandbox.SetInfraContainer(nil)

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("NetNsRemove", func() {
		It("should succeed when netns nil", func() {
			// Given
			// When
			err := testSandbox.NetNsRemove()

			// Then
			Expect(err).To(BeNil())
		})
	})

	t.Describe("NetNsJoin", func() {
		It("should fail when network namespace not exists", func() {
			// Given
			// When
			err := testSandbox.NetNsJoin("path", "name")

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("NetNsCreate", func() {
		It("should succeed", func() {
			// Given
			gomock.InOrder(
				netNsIfaceMock.EXPECT().Initialized().Return(false),
				netNsIfaceMock.EXPECT().Initialize().Return(netNsIfaceMock, nil),
				netNsIfaceMock.EXPECT().SymlinkCreate(gomock.Any()).Return(nil),
				netNsIfaceMock.EXPECT().Remove().Return(nil),
			)

			// When
			err := testSandbox.NetNsCreate(netNsIfaceMock)

			// Then
			Expect(err).To(BeNil())
			Expect(testSandbox.NetNsRemove()).To(BeNil())
		})

		It("should not crash when parameter is nil", func() {
			// Given
			// When
			_ = testSandbox.NetNsCreate(nil) // nolint: errcheck

			// Then
		})

		It("should fail on failed symlink creation", func() {
			// Given
			gomock.InOrder(
				netNsIfaceMock.EXPECT().Initialized().Return(false),
				netNsIfaceMock.EXPECT().Initialize().
					Return(netNsIfaceMock, nil),
				netNsIfaceMock.EXPECT().SymlinkCreate(gomock.Any()).
					Return(t.TestError),
				netNsIfaceMock.EXPECT().Close().Return(nil),
			)

			// When
			err := testSandbox.NetNsCreate(netNsIfaceMock)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail on failed symlink creation (with close error)", func() {
			// Given
			gomock.InOrder(
				netNsIfaceMock.EXPECT().Initialized().Return(false),
				netNsIfaceMock.EXPECT().Initialize().
					Return(netNsIfaceMock, nil),
				netNsIfaceMock.EXPECT().SymlinkCreate(gomock.Any()).
					Return(t.TestError),
				netNsIfaceMock.EXPECT().Close().Return(t.TestError),
			)

			// When
			err := testSandbox.NetNsCreate(netNsIfaceMock)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail on initialization error", func() {
			// Given
			gomock.InOrder(
				netNsIfaceMock.EXPECT().Initialized().Return(false),
				netNsIfaceMock.EXPECT().Initialize().
					Return(nil, t.TestError),
			)

			// When
			err := testSandbox.NetNsCreate(netNsIfaceMock)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail when already initialized", func() {
			// Given
			gomock.InOrder(
				netNsIfaceMock.EXPECT().Initialized().Return(true),
			)

			// When
			err := testSandbox.NetNsCreate(netNsIfaceMock)

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("HostNetNsPath", func() {
		It("should succeed", func() {
			// Given
			// When
			hostnet, err := sandbox.HostNetNsPath()

			// Then
			Expect(err).To(BeNil())
			Expect(hostnet).NotTo(BeNil())
		})
	})

	t.Describe("NetNsGet", func() {
		BeforeEach(func() {
			gomock.InOrder(
				netNsIfaceMock.EXPECT().Initialized().Return(false),
				netNsIfaceMock.EXPECT().Initialize().
					Return(netNsIfaceMock, nil),
				netNsIfaceMock.EXPECT().SymlinkCreate(gomock.Any()).
					Return(nil),
			)
			Expect(testSandbox.NetNsCreate(netNsIfaceMock)).To(BeNil())
		})

		It("should succeed", func() {
			// Given
			gomock.InOrder(
				netNsIfaceMock.EXPECT().Get().Return(&sandbox.NetNs{}),
			)
			// When
			ns, err := testSandbox.NetNsGet("/proc/self/ns",
				"../../../tmp/test")

			// Then
			defer os.RemoveAll(ns.Path())
			Expect(err).To(BeNil())
			Expect(ns).NotTo(BeNil())
			Expect(testSandbox.NetNs()).NotTo(BeNil())
			Expect(testSandbox.NetNsJoin("/proc/self/ns", ns.Path())).
				NotTo(BeNil())
			Expect(ns.Close()).To(BeNil())
			Expect(ns.Remove()).NotTo(BeNil())
		})

		It("should succeed with symlink", func() {
			// Given
			gomock.InOrder(
				netNsIfaceMock.EXPECT().Get().Return(&sandbox.NetNs{}),
			)
			const link = "ns-link"
			Expect(os.Symlink("/proc/self/ns", link)).To(BeNil())
			defer os.RemoveAll(link)

			// When
			ns, err := testSandbox.NetNsGet(link, "../../../tmp/test")

			// Then
			defer os.RemoveAll(ns.Path())
			Expect(err).To(BeNil())
			Expect(ns).NotTo(BeNil())
			Expect(testSandbox.NetNs()).NotTo(BeNil())
			Expect(ns.Remove()).NotTo(BeNil())
		})

		It("should fail on invalid namespace", func() {
			// Given

			// When
			ns, err := testSandbox.NetNsGet("", "")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(ns).To(BeNil())
		})
	})
})
