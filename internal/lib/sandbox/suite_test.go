package sandbox_test

import (
	"testing"
	"time"

	"github.com/cri-o/cri-o/internal/hostport"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	. "github.com/cri-o/cri-o/test/framework"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// TestSandbox runs the created specs
func TestSandbox(t *testing.T) {
	RegisterFailHandler(Fail)
	RunFrameworkSpecs(t, "Sandbox")
}

var (
	t           *TestFramework
	testSandbox *sandbox.Sandbox
)

var _ = BeforeSuite(func() {
	t = NewTestFramework(NilFunc, NilFunc)
	t.Setup()

	logrus.SetLevel(logrus.PanicLevel)
})

var _ = AfterSuite(func() {
	t.Teardown()
})

func beforeEach() {
	// Setup test vars
	var err error
	testSandbox, err = sandbox.New("sandboxID", "", "", "", "",
		make(map[string]string), make(map[string]string), "", "",
		&types.PodSandboxMetadata{}, "", "", false, "", "", "",
		[]*hostport.PortMapping{}, false, time.Now(), "", nil, nil)
	Expect(err).To(BeNil())
	Expect(testSandbox).NotTo(BeNil())
}
