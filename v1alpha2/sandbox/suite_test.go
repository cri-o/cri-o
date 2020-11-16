package sandbox_test

import (
	"context"
	"testing"

	. "github.com/cri-o/cri-o/test/framework"
	"github.com/cri-o/cri-o/v1alpha2/sandbox"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// TestSandbox runs the specs
func TestSandbox(t *testing.T) {
	RegisterFailHandler(Fail)
	RunFrameworkSpecs(t, "Sandbox")
}

var (
	t   *TestFramework
	sut sandbox.Sandbox
)

var _ = BeforeSuite(func() {
	t = NewTestFramework(NilFunc, NilFunc)
	t.Setup()
})

var _ = AfterSuite(func() {
	t.Teardown()
})

var _ = BeforeEach(func() {
	sut = sandbox.New(context.Background())
})
