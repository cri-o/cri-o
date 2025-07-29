//go:build !linux && !freebsd

package oci

import (
	"fmt"

	"github.com/cri-o/cri-o/internal/config/cgmgr"
)

// getContainerDiskStats is not supported on this platform.
func (r *runtimeOCI) getContainerDiskStats(c *Container) (*cgmgr.DiskMetrics, error) {
	return nil, fmt.Errorf("disk stats collection not supported on this platform")
}