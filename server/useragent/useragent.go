package useragent

import (
	"runtime"

	"github.com/cri-o/cri-o/internal/version"
)

// Get is the User-Agent the CRI-O daemon uses to identify itself.
func Get() string {
	httpVersion := make([]VersionInfo, 0, 4)
	httpVersion = append(httpVersion,
		VersionInfo{Name: "cri-o", Version: version.Version},
		VersionInfo{Name: "go", Version: runtime.Version()},
		VersionInfo{Name: "os", Version: runtime.GOOS},
		VersionInfo{Name: "arch", Version: runtime.GOARCH})

	return AppendVersions("", httpVersion...)
}
