package sandbox_test

import (
	"testing"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	. "github.com/cri-o/cri-o/test/framework"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
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
	testSandbox = sandbox.New()
	testSandbox.SetID("sandboxID")
	Expect(testSandbox).NotTo(BeNil())
}
