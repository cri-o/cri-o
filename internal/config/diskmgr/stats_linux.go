//go:build linux

package diskmgr

import (
	"fmt"
	"syscall"

	"github.com/cri-o/cri-o/utils"
)

type DiskMetrics struct {
	Filesystem FilesystemMetrics
}

// Filesystem Metrics represents disk usage statistics
type FilesystemMetrics struct {
	UsageBytes  uint64
	LimitBytes  uint64
	InodesFree  uint64
	InodesTotal uint64
}

// GetDiskUsageForPath returns disk usage statistics for a given path
func GetDiskUsageForPath(path string) (*DiskMetrics, error) {
	usageBytes, _, err := utils.GetDiskUsageStats(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get disk usage stats: %w", err)
	}

	// Get filesystem stats to determine total inodes and available space
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return nil, fmt.Errorf("failed to get filesystem stats: %w", err)
	}

	// Calculate free inodes and total inodes
	totalInodes := uint64(stat.Files)
	freeInodes := uint64(stat.Ffree)

	return &DiskMetrics{
		Filesystem: FilesystemMetrics{
			UsageBytes:  usageBytes,
			LimitBytes:  stat.Blocks * uint64(stat.Bsize),
			InodesFree:  freeInodes,
			InodesTotal: totalInodes,
		},
	}, nil
}
