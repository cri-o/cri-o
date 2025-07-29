//go:build freebsd

package oci

import (
	"fmt"
	"syscall"

	"github.com/cri-o/cri-o/internal/config/cgmgr"
)

// getContainerDiskStats collects disk metrics for a container on FreeBSD.
func (r *runtimeOCI) getContainerDiskStats(c *Container) (*cgmgr.DiskMetrics, error) {
	mountPoint := c.MountPoint()
	if mountPoint == "" {
		return nil, fmt.Errorf("container %s has no mount point", c.ID())
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(mountPoint, &stat); err != nil {
		return nil, fmt.Errorf("failed to get filesystem stats for %s: %w", mountPoint, err)
	}

	blockSize := uint64(stat.Bsize)
	totalBlocks := uint64(stat.Blocks)
	freeBlocks := uint64(stat.Bavail)

	usageBytes := (totalBlocks - freeBlocks) * blockSize
	limitBytes := totalBlocks * blockSize

	inodesTotal := uint64(stat.Files)
	inodesFree := uint64(stat.Ffree)

	return &cgmgr.DiskMetrics{
		UsageBytes:  usageBytes,
		LimitBytes:  limitBytes,
		InodesTotal: inodesTotal,
		InodesFree:  inodesFree,
	}, nil
}
