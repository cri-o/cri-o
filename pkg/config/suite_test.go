package config_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cri-o/cri-o/pkg/config"
	. "github.com/cri-o/cri-o/test/framework"
)

// TestLib runs the created specs.
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
	if errors.Is(err, exec.ErrNotFound) {
		Skip("conmon not found in $PATH")
	}

	Expect(err).ToNot(HaveOccurred())

	return conmonPath
}

var _ = BeforeSuite(func() {
	t = NewTestFramework(NilFunc, NilFunc)
	t.Setup()

	validDirPath = t.MustTempDir("crio-empty")

	useHermeticStorageConfig()
})

// useHermeticStorageConfig points containers/storage at a private, minimal
// configuration for the lifetime of the suite.
//
// Several specs in this suite run the storage-backed validation paths
// (Validate/ValidateRootConfig with onExecution) and compare the result against
// containers/storage' own defaults. Without this, they inherit the host's
// /etc/containers/storage.conf, so they only pass on hosts whose configured
// graph driver can actually be instantiated there. Inside a container that is
// typically not the case (overlay on top of overlayfs requires a mount program),
// which makes the specs report a storage failure that has nothing to do with the
// configuration logic under test.
//
// Pinning the driver to vfs plus private storage roots keeps the specs hermetic
// and meaningful: both the config under validation and the containers/storage
// defaults they are compared against come from the same, known configuration.
func useHermeticStorageConfig() {
	content := fmt.Sprintf("[storage]\n\tdriver = \"vfs\"\n\trunroot = %q\n\troot = %q\n",
		t.MustTempDir("crio-storage-run"), t.MustTempDir("crio-storage"))

	storageConf := t.MustTempFile("storage.conf")
	Expect(os.WriteFile(storageConf, []byte(content), 0o600)).To(Succeed())
	Expect(os.Setenv("CONTAINERS_STORAGE_CONF", storageConf)).To(Succeed())
}

var _ = AfterSuite(func() {
	t.Teardown()
})

func beforeEach() {
	sut = defaultConfig()
}

func defaultConfig() *config.Config {
	c, err := config.DefaultConfig()
	Expect(err).ToNot(HaveOccurred())
	Expect(c).NotTo(BeNil())
	t.EnsureRuntimeDeps()

	return c
}
