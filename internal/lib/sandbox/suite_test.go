package sandbox_test

import (
	"testing"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	. "github.com/cri-o/cri-o/test/framework"
	sandboxmock "github.com/cri-o/cri-o/test/mocks/sandbox"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"k8s.io/kubernetes/pkg/kubelet/dockershim/network/hostport"
)

// TestSandbox runs the created specs
func TestSandbox(t *testing.T) {
	RegisterFailHandler(Fail)
	RunFrameworkSpecs(t, "Sandbox")
}

var (
	t                  *TestFramework
	testSandbox        *sandbox.Sandbox
	mockCtrl           *gomock.Controller
	namespaceIfaceMock *sandboxmock.MockNamespaceIface
)

var _ = BeforeSuite(func() {
	t = NewTestFramework(NilFunc, NilFunc)
	t.Setup()

	logrus.SetLevel(logrus.PanicLevel)

	// Setup the mocks
	mockCtrl = gomock.NewController(GinkgoT())
	namespaceIfaceMock = sandboxmock.NewMockNamespaceIface(mockCtrl)
})

var _ = AfterSuite(func() {
	t.Teardown()
	mockCtrl.Finish()
})

func beforeEach() {
	// Setup test vars
	var err error
	testSandbox, err = sandbox.New("sandboxID", "", "", "", "",
		make(map[string]string), make(map[string]string), "", "",
		&pb.PodSandboxMetadata{}, "", "", false, "", "", "",
		[]*hostport.PortMapping{}, false)
	Expect(err).To(BeNil())
	Expect(testSandbox).NotTo(BeNil())
}
