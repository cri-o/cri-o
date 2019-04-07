package storage_test

import (
	"testing"

	. "github.com/cri-o/cri-o/test/framework"
	containerstoragemock "github.com/cri-o/cri-o/test/mocks/containerstorage"
	criostoragemock "github.com/cri-o/cri-o/test/mocks/criostorage"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// TestStorage runs the created specs
func TestStorage(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Storage")
}

// nolint: gochecknoglobals
var (
	t               *TestFramework
	mockCtrl        *gomock.Controller
	storeMock       *containerstoragemock.MockStore
	imageServerMock *criostoragemock.MockImageServer
	testManifest    []byte
)

var _ = BeforeSuite(func() {
	t = NewTestFramework(NilFunc, NilFunc)
	t.Setup()

	// Setup test data
	testManifest =
		[]byte(`{"schemaVersion": 1,"fsLayers":[{"blobSum": ""}],` +
			`"history": [{"v1Compatibility": "{\"id\":\"e45a5af57b00862e5ef57` +
			`82a9925979a02ba2b12dff832fd0991335f4a11e5c5\",\"parent\":\"\"}\n"}]}`)

	// Setup the mocks
	mockCtrl = gomock.NewController(GinkgoT())
	storeMock = containerstoragemock.NewMockStore(mockCtrl)
	imageServerMock = criostoragemock.NewMockImageServer(mockCtrl)
})

var _ = AfterSuite(func() {
	t.Teardown()
	mockCtrl.Finish()
})
