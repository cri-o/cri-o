package config_test

import (
	"os/exec"
	"testing"

	"github.com/cri-o/cri-o/pkg/config"
	. "github.com/cri-o/cri-o/test/framework"
	. "github.com/onsi/ginkgo/v2"
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
	invalidPath   = "/proc/invalid"
)

func validConmonPath() string {
	conmonPath, err := exec.LookPath("conmon")
	Expect(err).To(BeNil())
	return conmonPath
}

var _ = BeforeSuite(func() {
	t = NewTestFramework(NilFunc, NilFunc)
	t.Setup()

	validDirPath = t.MustTempDir("crio-empty")
})

var _ = AfterSuite(func() {
	t.Teardown()
})

func beforeEach() {
	sut = defaultConfig()
}

func defaultConfig() *config.Config {
	c, err := config.DefaultConfig()
	Expect(err).To(BeNil())
	Expect(c).NotTo(BeNil())
	return c
}
