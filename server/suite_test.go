package server_test

import (
	"context"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	cstorage "go.podman.io/storage"
	"go.uber.org/mock/gomock"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	"k8s.io/kubelet/pkg/cri/streaming"

	"github.com/cri-o/cri-o/internal/hostport"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/memorystore"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/pkg/config"
	"github.com/cri-o/cri-o/server"
	. "github.com/cri-o/cri-o/test/framework"
	imagetypesmock "github.com/cri-o/cri-o/test/mocks/containers/image/v5"
	containerstoragemock "github.com/cri-o/cri-o/test/mocks/containerstorage"
	criostoragemock "github.com/cri-o/cri-o/test/mocks/criostorage"
	libmock "github.com/cri-o/cri-o/test/mocks/lib"
	ocimock "github.com/cri-o/cri-o/test/mocks/oci"
	ocicnitypesmock "github.com/cri-o/cri-o/test/mocks/ocicni"
)

// TestServer runs the created specs.
func TestServer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunFrameworkSpecs(t, "Server")
}

var (
	libMock           *libmock.MockIface
	mockCtrl          *gomock.Controller
	serverConfig      *config.Config
	storeMock         *containerstoragemock.MockStore
	imageServerMock   *criostoragemock.MockImageServer
	runtimeServerMock *criostoragemock.MockRuntimeServer
	imageCloserMock   *imagetypesmock.MockImageCloser
	cniPluginMock     *ocicnitypesmock.MockCNIPlugin
	ociRuntimeMock    *ocimock.MockRuntimeImpl
	sut               *server.Server
	t                 *TestFramework
	testContainer     *oci.Container
	testManifest      []byte
	testPath          string
	testSandbox       *sandbox.Sandbox
	testStreamService *server.StreamService

	emptyDir string
)

const (
	sandboxID   = "sandboxID"
	containerID = "containerID"
)

var _ = BeforeSuite(func() {
	t = NewTestFramework(NilFunc, NilFunc)
	t.Setup()

	emptyDir = t.MustTempDir("crio-empty")
})

var _ = AfterSuite(func() {
	t.Teardown()
})

var beforeEach = func() {
	// Only log panics for now
	logrus.SetLevel(logrus.PanicLevel)

	// Setup the mocks
	mockCtrl = gomock.NewController(GinkgoT())
	libMock = libmock.NewMockIface(mockCtrl)
	storeMock = containerstoragemock.NewMockStore(mockCtrl)
	imageServerMock = criostoragemock.NewMockImageServer(mockCtrl)
	runtimeServerMock = criostoragemock.NewMockRuntimeServer(mockCtrl)
	imageCloserMock = imagetypesmock.NewMockImageCloser(mockCtrl)
	cniPluginMock = ocicnitypesmock.NewMockCNIPlugin(mockCtrl)
	ociRuntimeMock = ocimock.NewMockRuntimeImpl(mockCtrl)

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
			"io.kubernetes.cri-o.Image": "quay.io/image",
			"io.kubernetes.cri-o.ImageName": "example.com/some-other/deduplicated-name:notlatest",
			"io.kubernetes.cri-o.ImageRef": "1111111111111111111111111111111111111111111111111111111111111111",
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
			"io.kubernetes.cri-o.TrustedSandbox": "{}",
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

	// Prepare the server config
	var err error
	testPath, err = filepath.Abs("test")
	Expect(err).ToNot(HaveOccurred())
	serverConfig, err = config.DefaultConfig()
	Expect(err).ToNot(HaveOccurred())
	serverConfig.ContainerAttachSocketDir = testPath
	serverConfig.ContainerExitsDir = path.Join(testPath, "exits")
	serverConfig.LogDir = path.Join(testPath, "log")
	serverConfig.CleanShutdownFile = path.Join(testPath, "clean.shutdown")
	serverConfig.EnablePodEvents = true
	serverConfig.Seccomp().SetNotifierPath(t.MustTempDir("seccomp-notifier"))
	serverConfig.NRI.SocketPath = t.MustTempDir("nri")

	// Nothing here tests hostport mapping, and hostport.NewMetaHostPortManager
	// will fail if non-root
	serverConfig.DisableHostPortMapping = true

	// We want a directory that is guaranteed to exist, but it must
	// be empty so we don't erroneously load anything and make tests
	// unreproducible.
	serverConfig.NetworkDir = emptyDir
	serverConfig.PluginDirs = []string{emptyDir}
	serverConfig.HooksDir = []string{emptyDir}
	// Initialize test container and sandbox

	sbox := sandbox.NewBuilder()
	sbox.SetID("sandboxID")
	sbox.SetName("sandboxName")
	sbox.SetLogDir("test")
	sbox.SetShmPath("test")
	sbox.SetNamespace("")
	sbox.SetKubeName("")
	sbox.SetMountLabel("")
	sbox.SetProcessLabel("")
	sbox.SetCgroupParent("")
	sbox.SetRuntimeHandler("")
	sbox.SetResolvPath("")
	sbox.SetHostname("")
	sbox.SetPortMappings([]*hostport.PortMapping{})
	sbox.SetHostNetwork(false)
	sbox.SetUsernsMode("")
	sbox.SetPodLinuxOverhead(nil)
	sbox.SetPodLinuxResources(nil)
	sbox.SetCreatedAt(time.Now())
	err = sbox.SetCRISandbox(sbox.ID(), make(map[string]string), make(map[string]string), &types.PodSandboxMetadata{})
	Expect(err).ToNot(HaveOccurred())
	sbox.SetPrivileged(false)
	sbox.SetPortMappings([]*hostport.PortMapping{})
	sbox.SetHostNetwork(false)
	sbox.SetContainers(memorystore.New[*oci.Container]())
	testSandbox, err = sbox.GetSandbox()
	Expect(err).ToNot(HaveOccurred())

	testContainer, err = oci.NewContainer(containerID, "", "", "",
		make(map[string]string), make(map[string]string),
		make(map[string]string), "pauseImage", nil, nil, "",
		&types.ContainerMetadata{}, sandboxID, false, false,
		false, "", "", time.Now(), "")
	Expect(err).ToNot(HaveOccurred())

	// Initialize test streaming server
	streamServerConfig := streaming.DefaultConfig
	testStreamService = &server.StreamService{}
	testStreamService.SetRuntimeServer(sut)
	server, err := streaming.NewServer(streamServerConfig, testStreamService)
	Expect(err).ToNot(HaveOccurred())
	Expect(server).NotTo(BeNil())
}

var afterEach = func() {
	os.RemoveAll(testPath)
	os.RemoveAll("state.json")
	os.RemoveAll("config.json")
	mockCtrl.Finish()
}

var setupSUT = func() {
	var err error
	mockNewServer()
	sut, err = server.New(context.Background(), libMock)
	Expect(err).ToNot(HaveOccurred())
	Expect(sut).NotTo(BeNil())

	// Inject the mock
	sut.SetStorageImageServer(imageServerMock)
	sut.SetStorageRuntimeServer(runtimeServerMock)
}

func mockNewServer() {
	GinkgoHelper()
	gomock.InOrder(
		cniPluginMock.EXPECT().Status().Return(nil),
		libMock.EXPECT().GetData().Times(2).Return(serverConfig),
		libMock.EXPECT().GetStore().Return(storeMock, nil),
		libMock.EXPECT().GetData().Return(serverConfig),
		storeMock.EXPECT().Containers().
			Return([]cstorage.Container{}, nil),
		cniPluginMock.EXPECT().GC(gomock.Any(), gomock.Any()).
			Return(nil).AnyTimes(),
	)
	Expect(serverConfig.SetCNIPlugin(cniPluginMock)).To(Succeed())
}

func addContainerAndSandbox() {
	ctx := context.TODO()
	Expect(sut.AddSandbox(ctx, testSandbox)).To(Succeed())
	Expect(testSandbox.SetInfraContainer(testContainer)).To(Succeed())
	sut.AddContainer(ctx, testContainer)
	Expect(sut.CtrIDIndex().Add(testContainer.ID())).To(Succeed())
	Expect(sut.PodIDIndex().Add(testSandbox.ID())).To(Succeed())
	testContainer.SetCreated()
	testSandbox.SetCreated()
}

var mockDirs = func(manifest []byte) {
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

func createDummyState() {
	Expect(os.WriteFile("state.json", []byte(`{}`), 0o644)).To(Succeed())
}

func createDummyConfig() {
	Expect(os.WriteFile("config.json", []byte(`{"linux":{},"process":{}}`), 0o644)).To(Succeed())
}

func mockRuntimeInLibConfig() {
	echo, err := exec.LookPath("echo")
	Expect(err).ToNot(HaveOccurred())

	serverConfig.Runtimes[config.DefaultRuntime] = &config.RuntimeHandler{
		RuntimePath: echo,
	}
}
