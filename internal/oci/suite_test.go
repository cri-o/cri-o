package oci_test

import (
	"testing"
	"time"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/storage"
	"github.com/cri-o/cri-o/internal/storage/references"
	libconfig "github.com/cri-o/cri-o/pkg/config"
	. "github.com/cri-o/cri-o/test/framework"
	containerstoragemock "github.com/cri-o/cri-o/test/mocks/containerstorage"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// TestOci runs the created specs
func TestOci(t *testing.T) {
	RegisterFailHandler(Fail)
	RunFrameworkSpecs(t, "Oci")
}

var (
	t           *TestFramework
	mockCtrl    *gomock.Controller
	storeMock   *containerstoragemock.MockStore
	myContainer *oci.Container
	config      *libconfig.Config
)

const (
	sandboxID   = "sandboxID"
	containerID = "containerID"
)

func beforeEach() {
	var err error
	myContainer, err = oci.NewContainer(containerID, "", "", "",
		make(map[string]string), make(map[string]string),
		make(map[string]string), "", nil, nil, "",
		&types.ContainerMetadata{}, sandboxID, false,
		false, false, "", "", time.Now(), "")
	Expect(err).To(BeNil())
}

var _ = BeforeSuite(func() {
	t = NewTestFramework(NilFunc, NilFunc)
	t.Setup()

	// Setup the mocks
	mockCtrl = gomock.NewController(GinkgoT())
	storeMock = containerstoragemock.NewMockStore(mockCtrl)
})

func getTestContainer() *oci.Container {
	imageName, err := references.ParseRegistryImageReferenceFromOutOfProcessData("docker.io/library/image-name:latest")
	Expect(err).To(BeNil())
	imageID, err := storage.ParseStorageImageIDFromOutOfProcessData("2a03a6059f21e150ae84b0973863609494aad70f0a80eaeb64bddd8d92465812")
	Expect(err).To(BeNil())
	container, err := oci.NewContainer("id", "name", "bundlePath", "logPath",
		map[string]string{"key": "label"},
		map[string]string{"key": "crioAnnotation"},
		map[string]string{"key": "annotation"},
		"image", &imageName, &imageID, "", &types.ContainerMetadata{}, "sandbox",
		false, false, false, "", "dir", time.Now(), "")
	Expect(err).To(BeNil())
	Expect(container).NotTo(BeNil())
	return container
}

var _ = AfterSuite(func() {
	t.Teardown()
	mockCtrl.Finish()
})
