package main

import (
	"bytes"
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cri-o/cri-o/scripts/utils"
)

var _ = t.Describe("Automated Releases", func() {
	t.Describe("Patch Release", func() {
		It("should read the version file", func() {
			tempFileName := "tempVersionFile"
			tmpfile, err := os.CreateTemp("", tempFileName)
			if err != nil {
				Expect(err).ToNot(HaveOccurred())
			}
			defer os.Remove(tempFileName)
			currentVersion := "1.30.0"
			originalFileContent := getMockVersionFileContent(currentVersion)

			if _, err := tmpfile.Write(originalFileContent); err != nil {
				tmpfile.Close()
				Expect(err).ToNot(HaveOccurred())
			}
			if err := tmpfile.Close(); err != nil {
				Expect(err).ToNot(HaveOccurred())
			}

			versionFromFile, err := utils.GetCurrentVersionFromVersionFile(tmpfile.Name())
			if err != nil {
				Expect(err).ToNot(HaveOccurred())
			}

			areVersionsTheSame := currentVersion == versionFromFile

			Expect(areVersionsTheSame).To(BeTrue())
		})
		It("should modify the version file", func() {
			tempFileName := "tempVersionFile"
			tmpfile, err := os.CreateTemp("", tempFileName)
			if err != nil {
				Expect(err).ToNot(HaveOccurred())
			}
			defer os.Remove(tempFileName)
			oldVersion := "1.30.0"
			newVersion := "1.30.1"

			originalFileContent := getMockVersionFileContent(oldVersion)
			if _, err := tmpfile.Write(originalFileContent); err != nil {
				tmpfile.Close()
				Expect(err).ToNot(HaveOccurred())
			}
			if err := tmpfile.Close(); err != nil {
				Expect(err).ToNot(HaveOccurred())
			}

			if err := modifyVersionFile(tmpfile.Name(), oldVersion, newVersion); err != nil {
				Expect(err).ToNot(HaveOccurred())
			}

			modifiedContent, err := os.ReadFile(tmpfile.Name())
			if err != nil {
				Expect(err).ToNot(HaveOccurred())
			}

			expectedModifiedContent := getMockVersionFileContent(newVersion)
			if !bytes.Equal(modifiedContent, expectedModifiedContent) {
				Expect(err).ToNot(HaveOccurred())
			}
		})
	})
})

func getMockVersionFileContent(version string) []byte {
	return []byte(fmt.Sprintf(`
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
    "github.com/containers/common/pkg/apparmor"
    "github.com/containers/common/pkg/seccomp"
    "github.com/google/renameio"
    json "github.com/json-iterator/go"
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
  `, version))
}
