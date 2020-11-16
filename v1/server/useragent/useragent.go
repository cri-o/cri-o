package useragent

import (
	"runtime"

	"github.com/cri-o/cri-o/internal/version"
)

// Get is the User-Agent the CRI-O daemon uses to identify itself.
func Get() string {
	info := version.Get()
	httpVersion := make([]VersionInfo, 0, 4)
	httpVersion = append(httpVersion,
		VersionInfo{Name: "cri-o", Version: info.Version},
		VersionInfo{Name: "go", Version: info.GoVersion},
		VersionInfo{Name: "os", Version: runtime.GOOS},
		VersionInfo{Name: "arch", Version: runtime.GOARCH})

	return AppendVersions("", httpVersion...)
}
