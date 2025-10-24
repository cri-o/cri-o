//go:build !linux && !freebsd

package oci

import (
	"fmt"

	"github.com/cri-o/cri-o/internal/config/diskmgr"
)

// getContainerDiskStats is not supported on this platform.
func (r *runtimeOCI) getContainerDiskStats(c *Container) (*diskmgr.DiskMetrics, error) {
	return nil, fmt.Errorf("disk stats collection not supported on this platform")
}
