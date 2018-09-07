package useragent

import (
	"context"
	"runtime"

	"github.com/kubernetes-sigs/cri-o/version"
)

// Get is the User-Agent the CRI-O daemon uses to identify itself.
func Get(ctx context.Context) string {
	httpVersion := make([]VersionInfo, 0, 4)
	httpVersion = append(httpVersion, VersionInfo{Name: "cri-o", Version: version.Version})
	httpVersion = append(httpVersion, VersionInfo{Name: "go", Version: runtime.Version()})
	httpVersion = append(httpVersion, VersionInfo{Name: "os", Version: runtime.GOOS})
	httpVersion = append(httpVersion, VersionInfo{Name: "arch", Version: runtime.GOARCH})

	return AppendVersions("", httpVersion...)
}
