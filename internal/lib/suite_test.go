package lib_test

import (
	"context"
	"os"
	"testing"
	"time"

	cstorage "github.com/containers/storage"
	"github.com/cri-o/cri-o/internal/hostport"
	"github.com/cri-o/cri-o/internal/lib"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/oci"
	libconfig "github.com/cri-o/cri-o/pkg/config"
	. "github.com/cri-o/cri-o/test/framework"
	containerstoragemock "github.com/cri-o/cri-o/test/mocks/containerstorage"
	criostoragemock "github.com/cri-o/cri-o/test/mocks/criostorage"
	libmock "github.com/cri-o/cri-o/test/mocks/lib"
	ocimock "github.com/cri-o/cri-o/test/mocks/oci"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// TestLib runs the created specs
func TestLib(t *testing.T) {
	RegisterFailHandler(Fail)
	RunFrameworkSpecs(t, "Lib")
}

var (
	t                    *TestFramework
	config               *libconfig.Config
	mockCtrl             *gomock.Controller
	libMock              *libmock.MockIface
	storeMock            *containerstoragemock.MockStore
	multiStoreServerMock *criostoragemock.MockMultiStoreServer
	ociRuntimeMock       *ocimock.MockRuntimeImpl
	ociRuntimeMock       *ocimock.MockRuntimeImpl
	testManifest         []byte
	sut                  *lib.ContainerServer
	mySandbox            *sandbox.Sandbox
	myContainer          *oci.Container

	validDirPath string
)

const (
	sandboxID   = "sandboxID"
	containerID = "containerID"
)

var _ = BeforeSuite(func() {
	t = NewTestFramework(NilFunc, NilFunc)
	t.Setup()

	// Setup test data
	testManifest = []byte(`{
		"annotations": {
			"io.kubernetes.cri-o.Annotations": "{}",
			"io.kubernetes.cri-o.ContainerID": "sandboxID",
			"io.kubernetes.cri-o.ContainerName": "containerName",
			"io.kubernetes.cri-o.ContainerType": "{}",
			"io.kubernetes.cri-o.Created": "2006-01-02T15:04:05.999999999Z",
			"io.kubernetes.cri-o.HostName": "{}",
			"io.kubernetes.cri-o.CgroupParent": "{}",
			"io.kubernetes.cri-o.IP": "{}",
			"io.kubernetes.cri-o.NamespaceOptions": "{}",
			"io.kubernetes.cri-o.SeccompProfilePath": "{}",
			"io.kubernetes.cri-o.Image": "{}",
			"io.kubernetes.cri-o.ImageName": "{}",
			"io.kubernetes.cri-o.ImageRef": "{}",
			"io.kubernetes.cri-o.KubeName": "{}",
			"io.kubernetes.cri-o.PortMappings": "[]",
			"io.kubernetes.cri-o.Labels": "{}",
			"io.kubernetes.cri-o.LogPath": "{}",
			"io.kubernetes.cri-o.Metadata": "{}",
			"io.kubernetes.cri-o.Name": "name",
			"io.kubernetes.cri-o.Namespace": "default",
			"io.kubernetes.cri-o.PrivilegedRuntime": "{}",
			"io.kubernetes.cri-o.ResolvPath": "{}",
			"io.kubernetes.cri-o.HostnamePath": "{}",
			"io.kubernetes.cri-o.SandboxID": "sandboxID",
			"io.kubernetes.cri-o.SandboxName": "{}",
			"io.kubernetes.cri-o.ShmPath": "{}",
			"io.kubernetes.cri-o.MountPoint": "{}",
			"io.kubernetes.cri-o.Stdin": "{}",
			"io.kubernetes.cri-o.StdinOnce": "{}",
			"io.kubernetes.cri-o.Volumes": "[{}]",
			"io.kubernetes.cri-o.HostNetwork": "{}",
			"io.kubernetes.cri-o.CNIResult": "{}"
		},
		"linux": {
			"namespaces": [
				{"type": "network", "path": "/proc/self/ns/net"}
			]
		},
		"process": {
			"selinuxLabel": "system_u:system_r:container_runtime_t:s0"
		}}`)

	// Setup the mocks
	mockCtrl = gomock.NewController(GinkgoT())
	libMock = libmock.NewMockIface(mockCtrl)
	storeMock = containerstoragemock.NewMockStore(mockCtrl)
	multiStoreMock = criostoragemock.NewMockMultiStore(mockCtrl)
	multiStoreServerMock = criostoragemock.NewMockMultiStoreServer(mockCtrl)
	ociRuntimeMock = ocimock.NewMockRuntimeImpl(mockCtrl)

	validDirPath = t.MustTempDir("crio-empty")
})

var _ = AfterSuite(func() {
	removeConfig()
	t.Teardown()
	mockCtrl.Finish()
	_ = os.RemoveAll("/tmp/fake-runtime")
})

func removeState() {
	_ = os.RemoveAll("state.json")
}

func removeConfig() {
	_ = os.RemoveAll("config.json")
}

func beforeEach() {
	// Remove old state files
	removeState()
	// Remove old config files
	removeConfig()

	// Only log panics for now
	logrus.SetLevel(logrus.PanicLevel)

	// Set the config
	var err error
	config, err = libconfig.DefaultConfig()
	Expect(err).To(BeNil())
	config.LogDir = "."
	config.HooksDir = []string{}
	// so we have permission to make a directory within it
	config.ContainerAttachSocketDir = t.MustTempDir("crio")

	store := map[string]cstorage.Store{"defaultStorage": storeMock}

	gomock.InOrder(
		libMock.EXPECT().GetStore().Return(multiStoreMock, nil),
		libMock.EXPECT().GetData().Return(config),
		multiStoreMock.EXPECT().GetStore().Return(store),
		libMock.EXPECT().GetData().Return(config),
	)

	// Setup the sut
	sut, err = lib.New(context.Background(), libMock)
	Expect(err).To(BeNil())
	Expect(sut).NotTo(BeNil())

	// Setup test vars
	mySandbox, err = sandbox.New(sandboxID, "", "", "", "",
		make(map[string]string), make(map[string]string), "", "",
		&types.PodSandboxMetadata{}, "", "", false, "", "", "",
		[]*hostport.PortMapping{}, false, time.Now(), "")
	Expect(err).To(BeNil())

	myContainer, err = oci.NewContainer(containerID, "", "", "",
		make(map[string]string), make(map[string]string),
		make(map[string]string), "", "", "",
		&types.ContainerMetadata{}, sandboxID, false,
		false, false, "", "", time.Now(), "")
	Expect(err).To(BeNil())
}

func mockDirs(manifest []byte) {
	gomock.InOrder(
		multiStoreMock.EXPECT().GetDefaultStorageDriver().Return("defaultStorage"),
		storeMock.EXPECT().Container(gomock.Any()).Return(nil, nil),
		storeMock.EXPECT().
			FromContainerDirectory(gomock.Any(), gomock.Any()).
			Return(manifest, nil),
		multiStoreMock.EXPECT().GetDefaultStorageDriver().Return("defaultStorage"),
		storeMock.EXPECT().Container(gomock.Any()).Return(nil, nil),
		storeMock.EXPECT().ContainerRunDirectory(gomock.Any()).
			Return("", nil),
		multiStoreMock.EXPECT().GetDefaultStorageDriver().Return("defaultStorage"),
		storeMock.EXPECT().Container(gomock.Any()).Return(nil, nil),
		storeMock.EXPECT().ContainerDirectory(gomock.Any()).
			Return("", nil),
	)
}

func addContainerAndSandbox() {
	Expect(sut.AddSandbox(mySandbox)).To(BeNil())
	sut.AddContainer(myContainer)
	Expect(sut.CtrIDIndex().Add(containerID)).To(BeNil())
	Expect(sut.PodIDIndex().Add(sandboxID)).To(BeNil())
	myContainer.SetCreated()
}

func createDummyState() {
	Expect(os.WriteFile("state.json", []byte("{}"), 0o644)).To(BeNil())
}

func createDummyConfig() {
	Expect(os.WriteFile("config.json", []byte(`{"linux":{},"process":{}}`), 0o644)).To(BeNil())
}

func mockRuncInLibConfig() {
	config.Runtimes["runc"] = &libconfig.RuntimeHandler{
		RuntimePath: "/bin/echo",
	}
}

func mockRuncInLibConfigCheckpoint() {
	Expect(os.WriteFile("/tmp/fake-runtime", []byte("#!/bin/bash\n\necho flag needs an argument\nexit 0\n"), 0o755)).To(BeNil())
	config.Runtimes["runc"] = &libconfig.RuntimeHandler{
		RuntimePath: "/tmp/fake-runtime",
		MonitorPath: "/bin/true",
	}
}

func mockRuncToFalseInLibConfig() {
	config.Runtimes["runc"] = &libconfig.RuntimeHandler{
		RuntimePath: "/bin/false",
	}
}
