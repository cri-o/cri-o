package references_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/cri-o/cri-o/test/framework"
)

// TestReferences runs the created specs.
func TestReferences(t *testing.T) {
	RegisterFailHandler(Fail)
	RunFrameworkSpecs(t, "Storage/references")
}

var t *TestFramework

var _ = BeforeSuite(func() {
	t = NewTestFramework(NilFunc, NilFunc)
	t.Setup()
})

var _ = AfterSuite(func() {
	t.Teardown()
})
