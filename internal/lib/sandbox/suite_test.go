package sandbox_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/hostport"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/oci"
	. "github.com/cri-o/cri-o/test/framework"
)

// TestSandbox runs the created specs.
func TestSandbox(t *testing.T) {
	RegisterFailHandler(Fail)
	RunFrameworkSpecs(t, "Sandbox")
}

var (
	t           *TestFramework
	testSandbox *sandbox.Sandbox
	builder     sandbox.Builder
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
	sbuilder := sandbox.NewBuilder()
	sbuilder.SetID("sandboxID")
	sbuilder.SetName("")
	sbuilder.SetNamespace("")
	sbuilder.SetKubeName("")
	sbuilder.SetLogDir("test")
	sbuilder.SetCriSandbox(sbuilder.ID(), time.Now(), make(map[string]string), make(map[string]string), &types.PodSandboxMetadata{})
	sbuilder.SetShmPath("")
	sbuilder.SetCgroupParent("")
	sbuilder.SetPrivileged(false)
	sbuilder.SetRuntimeHandler("")
	sbuilder.SetResolvPath("")
	sbuilder.SetHostname("")
	sbuilder.SetPortMappings([]*hostport.PortMapping{})
	sbuilder.SetHostNetwork(false)
	sbuilder.SetUsernsMode("")
	sbuilder.SetPodLinuxOverhead(nil)
	sbuilder.SetPodLinuxResources(nil)
	sbuilder.SetContainers(oci.NewMemoryStore())
	sbuilder.SetProcessLabel("")
	sbuilder.SetMountLabel("")

	testSandbox = sbuilder.GetSandbox()

	builder = sandbox.NewBuilder()
	Expect(testSandbox).NotTo(BeNil())
}
