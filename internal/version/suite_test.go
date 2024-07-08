package version

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/cri-o/cri-o/test/framework"
)

// TestVersion runs the created specs.
func TestVersion(t *testing.T) {
	RegisterFailHandler(Fail)
	RunFrameworkSpecs(t, "Version")
}

var t *TestFramework

var _ = BeforeSuite(func() {
	t = NewTestFramework(NilFunc, NilFunc)
	t.Setup()
})

var _ = AfterSuite(func() {
	t.Teardown()
})
