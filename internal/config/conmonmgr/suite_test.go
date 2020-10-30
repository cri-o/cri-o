package conmonmgr

import (
	"testing"

	. "github.com/cri-o/cri-o/test/framework"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// TestLib runs the created specs
func TestLibConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunFrameworkSpecs(t, "ConmonManager")
}

var (
	t        *TestFramework
	mockCtrl *gomock.Controller
)

var _ = BeforeSuite(func() {
	t = NewTestFramework(NilFunc, NilFunc)
	t.Setup()
	mockCtrl = gomock.NewController(GinkgoT())
})

var _ = AfterSuite(func() {
	t.Teardown()
})
