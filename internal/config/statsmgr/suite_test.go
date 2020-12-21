package statsmgr_test

import (
	"testing"

	. "github.com/cri-o/cri-o/test/framework"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// TestStatsManager runs the created specs
func TestStatsManager(t *testing.T) {
	RegisterFailHandler(Fail)
	RunFrameworkSpecs(t, "StatsManager")
}

var t *TestFramework

var _ = BeforeSuite(func() {
	t = NewTestFramework(NilFunc, NilFunc)
	t.Setup()
})

var _ = AfterSuite(func() {
	t.Teardown()
})
