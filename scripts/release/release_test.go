package main

import (
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cri-o/cri-o/scripts/utils"
)

var _ = t.Describe("Automated Releases", func() {
	t.Describe("Patch Release", func() {
		It("should read the version file", func() {
			tmpfile, err := os.CreateTemp("", "tempVersionFile")
			Expect(err).ToNot(HaveOccurred())

			defer os.Remove(tmpfile.Name())
			defer tmpfile.Close()

			currentVersion := "1.30.0"
			originalFileContent := getMockVersionFileContent(currentVersion)

			_, err = tmpfile.Write(originalFileContent)
			Expect(err).ToNot(HaveOccurred())
			Expect(tmpfile.Close()).To(Succeed())

			versionFromFile, err := utils.GetCurrentVersionFromVersionFile(tmpfile.Name())
			Expect(err).ToNot(HaveOccurred())
			Expect(versionFromFile).To(Equal(currentVersion))
		})
		It("should modify the version file", func() {
			tmpfile, err := os.CreateTemp("", "tempVersionFile")
			Expect(err).ToNot(HaveOccurred())

			defer os.Remove(tmpfile.Name())
			defer tmpfile.Close()

			oldVersion := "1.30.0"
			newVersion := "1.30.1"

			originalFileContent := getMockVersionFileContent(oldVersion)
			_, err = tmpfile.Write(originalFileContent)
			Expect(err).ToNot(HaveOccurred())
			Expect(tmpfile.Close()).To(Succeed())
			Expect(modifyVersionFile(tmpfile.Name(), oldVersion, newVersion)).To(Succeed())

			modifiedContent, err := os.ReadFile(tmpfile.Name())
			Expect(err).ToNot(HaveOccurred())

			expectedModifiedContent := getMockVersionFileContent(newVersion)
			Expect(modifiedContent).To(Equal(expectedModifiedContent))
		})
	})
})

func getMockVersionFileContent(version string) []byte {
	return fmt.Appendf(nil, `
  package version

  import (
    "errors"
    "fmt"
    "os"
    "path/filepath"
    "reflect"
    "runtime"
    "runtime/debug"
    "strconv"
    "strings"
    "text/tabwriter"

    "github.com/blang/semver/v4"
    "go.podman.io/common/pkg/apparmor"
    "go.podman.io/common/pkg/seccomp"
    "github.com/google/renameio"
    json "github.com/goccy/go-json"
    "github.com/sirupsen/logrus"
  )

  // Version is the version of the build.
  const Version = "%s"

  // Variables injected during build-time
  var (
    buildDate string
  )

  // ShouldCrioWipe opens the version file, and parses it and the version string
  // If there is a parsing error, then crio should wipe, and the error is returned.
  // if parsing is successful, it compares the major and minor versions
  // and returns whether the major and minor versions are the same.
  // If they differ, then crio should wipe.
  func ShouldCrioWipe(versionFileName string) (bool, error) {
    return shouldCrioWipe(versionFileName, Version)
  }
  `, version)
}
