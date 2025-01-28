package memorystore_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/hostport"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	. "github.com/cri-o/cri-o/test/framework"
)

// TestMemoryStore runs the created specs.
func TestMemoryStore(t *testing.T) {
	RegisterFailHandler(Fail)
	RunFrameworkSpecs(t, "MemoryStore")
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

	createdAt := time.Now()
	sbox := sandbox.NewBuilder()
	sbox.SetID("sandboxID")
	sbox.SetLogDir("test")
	sbox.SetCreatedAt(createdAt)
	err = sbox.SetCRISandbox(sbox.ID(), make(map[string]string), make(map[string]string), &types.PodSandboxMetadata{})
	Expect(err).ToNot(HaveOccurred())
	sbox.SetPrivileged(false)
	sbox.SetPortMappings([]*hostport.PortMapping{})
	sbox.SetHostNetwork(false)
	sbox.SetCreatedAt(createdAt)

	testSandbox, err = sbox.GetSandbox()
	Expect(err).ToNot(HaveOccurred())
	Expect(err).ToNot(HaveOccurred())
	Expect(testSandbox).NotTo(BeNil())
}
