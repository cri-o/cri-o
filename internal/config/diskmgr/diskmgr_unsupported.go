//go:build !linux

package diskmgr

import "errors"

// DiskMetrics represents disk usage statistics
type DiskManager interface {
	// Name returns the name of the disk manager
	Name() string
	// ContainerDiskAbsolutePath returns the absolute on-disk path of the container's disk cgroup
	ContainerDiskAbsolutePath(sbParent, containerID string) (string, error)
	// ContainerDiskStats returns disk I/O stats for the given container, creating cgroup if necessary
	ContainerDiskStats(sbParent, containerID string) (*DiskMetrics, error)
}

type NullDiskManager struct{}

// New creates a new DiskManager with defaults
func New() DiskManager {
	return &NullDiskManager{}
}

// ContainerDiskAbsolutePath implements DiskManager.
func (n *NullDiskManager) ContainerDiskAbsolutePath(sbParent string, containerID string) (string, error) {
	panic("unimplemented")
}

// ContainerDiskStats implements DiskManager.
func (n *NullDiskManager) ContainerDiskStats(sbParent string, containerID string) (*DiskMetrics, error) {
	panic("unimplemented")
}

// Name implements DiskManager.
func (n *NullDiskManager) Name() string {
	panic("unimplemented")
}

func InitializeDiskManager() (DiskManager, error) {
	return nil, errors.New("not implemented yet")
}
