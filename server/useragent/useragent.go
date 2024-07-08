package useragent

import (
	"fmt"
	"regexp"
	"runtime"

	"github.com/cri-o/cri-o/internal/version"
)

// Simplest semver "X.Y.Z" format.
var versionRegex = regexp.MustCompile(`(\d+\.\d+\.\d+)`)

// Get is the User-Agent the CRI-O daemon uses to identify itself.
func Get() (string, error) {
	info, err := version.Get(false)
	if err != nil {
		return "", fmt.Errorf("get version: %w", err)
	}

	// Ensure that the CRI-O version set in the User-Agent header
	// is always of the simplest semver format, and remove everything
	// else that might have been added as part of the build process.
	versionString := info.Version
	if s := versionRegex.FindString(versionString); s != "" {
		versionString = s
	}

	httpVersion := AppendVersions("", []VersionInfo{
		{Name: "cri-o", Version: versionString},
		{Name: "go", Version: info.GoVersion},
		{Name: "os", Version: runtime.GOOS},
		{Name: "arch", Version: runtime.GOARCH},
	}...)

	return httpVersion, nil
}
