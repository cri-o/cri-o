package useragent

import (
	"runtime"

	"github.com/cri-o/cri-o/internal/version"
	"github.com/pkg/errors"
)

// Get is the User-Agent the CRI-O daemon uses to identify itself.
func Get() (string, error) {
	info, err := version.Get(false)
	if err != nil {
		return "", errors.Wrap(err, "get version")
	}
	httpVersion := make([]VersionInfo, 0, 4)
	httpVersion = append(httpVersion,
		VersionInfo{Name: "cri-o", Version: info.Version},
		VersionInfo{Name: "go", Version: info.GoVersion},
		VersionInfo{Name: "os", Version: runtime.GOOS},
		VersionInfo{Name: "arch", Version: runtime.GOARCH})

	return AppendVersions("", httpVersion...), nil
}
