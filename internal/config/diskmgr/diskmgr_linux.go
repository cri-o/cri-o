//go:build linux

package diskmgr

import (
	"os"
)

const (
	CrioPrefix = "crio"
)

// DiskManager defines an interface for disk statistics collection
type DiskManager interface {
	// Name returns the name of the disk manager
	Name() string
	// ContainerDiskPath returns the container’s mount path used for measuring disk usage
	ContainerDiskPath(containerMountPath string) string
	// ContainerDiskStats returns disk statistics for the given container mount path
	ContainerDiskStats(containerMountPath string) (*DiskMetrics, error)
}

// diskManagerImpl is a basic implementation that uses filesystem statistics
type diskManagerImpl struct{}

// New returns a new DiskManager instance
func New() DiskManager {
	return &diskManagerImpl{}
}

// Name returns the name of this disk manager
func (d *diskManagerImpl) Name() string {
	return CrioPrefix
}

// ContainerDiskPath returns the given mount path directly
func (d *diskManagerImpl) ContainerDiskPath(containerMountPath string) string {
	return containerMountPath
}

// ContainerDiskStats calculates disk usage for the given path
func (d *diskManagerImpl) ContainerDiskStats(containerMountPath string) (*DiskMetrics, error) {
	// Ensure the path exists
	if _, err := os.Stat(containerMountPath); err != nil {
		return nil, err
	}

	// Collect disk usage stats
	return GetDiskUsageForPath(containerMountPath)
}
