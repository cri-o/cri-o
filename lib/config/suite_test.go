package config_test

import (
	"testing"

	"github.com/cri-o/cri-o/lib/config"
	. "github.com/cri-o/cri-o/test/framework"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// TestLib runs the created specs
func TestLibConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunFrameworkSpecs(t, "LibConfig")
}

var (
	t            *TestFramework
	sut          *config.Config
	validDirPath string
)

const (
	validFilePath = "/bin/sh"
	invalidPath   = "/wrong"
)

var _ = BeforeSuite(func() {
	t = NewTestFramework(NilFunc, NilFunc)
	t.Setup()

	validDirPath = t.MustTempDir("crio-empty")
})

var _ = AfterSuite(func() {
	t.Teardown()
})

func beforeEach() {
	var err error
	sut, err = config.DefaultConfig()
	Expect(err).To(BeNil())
	Expect(sut).NotTo(BeNil())
}
