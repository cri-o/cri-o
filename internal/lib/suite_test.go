package lib_test

import (
	"context"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/cri-o/cri-o/internal/lib"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/oci"
	libconfig "github.com/cri-o/cri-o/pkg/config"
	. "github.com/cri-o/cri-o/test/framework"
	containerstoragemock "github.com/cri-o/cri-o/test/mocks/containerstorage"
	libmock "github.com/cri-o/cri-o/test/mocks/lib"
	ocimock "github.com/cri-o/cri-o/test/mocks/oci"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"k8s.io/kubernetes/pkg/kubelet/dockershim/network/hostport"
)

// TestLib runs the created specs
func TestLib(t *testing.T) {
	RegisterFailHandler(Fail)
	RunFrameworkSpecs(t, "Lib")
}

var (
	t              *TestFramework
	mockCtrl       *gomock.Controller
	libMock        *libmock.MockIface
	storeMock      *containerstoragemock.MockStore
	ociRuntimeMock *ocimock.MockRuntimeImpl
	testManifest   []byte
	sut            *lib.ContainerServer
	mySandbox      *sandbox.Sandbox
	myContainer    *oci.Container

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
				{"type": "network", "path": "default"}
			]
		},
		"process": {
			"selinuxLabel": "system_u:system_r:container_runtime_t:s0"
		}}`)

	// Setup the mocks
	mockCtrl = gomock.NewController(GinkgoT())
	libMock = libmock.NewMockIface(mockCtrl)
	storeMock = containerstoragemock.NewMockStore(mockCtrl)
	ociRuntimeMock = ocimock.NewMockRuntimeImpl(mockCtrl)

	validDirPath = t.MustTempDir("crio-empty")
})

var _ = AfterSuite(func() {
	t.Teardown()
	mockCtrl.Finish()
})

func removeState() {
	_ = os.RemoveAll("state.json")
}

func beforeEach() {
	// Remove old state files
	removeState()

	// Only log panics for now
	logrus.SetLevel(logrus.PanicLevel)

	// Set the config
	config, err := libconfig.DefaultConfig()
	Expect(err).To(BeNil())
	config.LogDir = "."
	config.HooksDir = []string{}

	gomock.InOrder(
		libMock.EXPECT().GetStore().Return(storeMock, nil),
		libMock.EXPECT().GetData().Return(config),
	)

	// Setup the sut
	sut, err = lib.New(context.Background(), libMock)
	Expect(err).To(BeNil())
	Expect(sut).NotTo(BeNil())

	// Setup test vars
	mySandbox, err = sandbox.New(sandboxID, "", "", "", "",
		make(map[string]string), make(map[string]string), "", "",
		&pb.PodSandboxMetadata{}, "", "", false, "", "", "",
		[]*hostport.PortMapping{}, false)
	Expect(err).To(BeNil())

	myContainer, err = oci.NewContainer(containerID, "", "", "",
		make(map[string]string), make(map[string]string),
		make(map[string]string), "", "", "",
		&pb.ContainerMetadata{}, sandboxID, false,
		false, false, "", "", time.Now(), "")
	Expect(err).To(BeNil())
}

func mockDirs(manifest []byte) {
	gomock.InOrder(
		storeMock.EXPECT().
			FromContainerDirectory(gomock.Any(), gomock.Any()).
			Return(manifest, nil),
		storeMock.EXPECT().ContainerRunDirectory(gomock.Any()).
			Return("", nil),
		storeMock.EXPECT().ContainerDirectory(gomock.Any()).
			Return("", nil),
	)
}

func addContainerAndSandbox() {
	Expect(sut.AddSandbox(mySandbox)).To(BeNil())
	sut.AddContainer(myContainer)
	Expect(sut.CtrIDIndex().Add(containerID)).To(BeNil())
	Expect(sut.PodIDIndex().Add(sandboxID)).To(BeNil())
}

func createDummyState() {
	Expect(ioutil.WriteFile("state.json", []byte("{}"), 0644)).To(BeNil())
}
