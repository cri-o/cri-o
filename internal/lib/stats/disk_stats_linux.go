package stats

import (
	"fmt"
	"syscall"

	"github.com/cri-o/cri-o/utils"
)

// DiskStats represents comprehensive disk statistics for a container.
type DiskStats struct {
	Filesystem FilesystemStats
}

// FilesystemStats represents filesystem usage statistics.
type FilesystemStats struct {
	UsageBytes  uint64 `json:"usage_bytes"`
	LimitBytes  uint64 `json:"limit_bytes"`
	InodesFree  uint64 `json:"inodes_free"`
	InodesTotal uint64 `json:"inodes_total"`
}

// GetDiskUsageForPath returns disk usage statistics for a given path.
func GetDiskUsageForPath(path string) (*DiskStats, error) {
	usageBytes, _, err := utils.GetDiskUsageStats(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get disk usage stats: %w", err)
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return nil, fmt.Errorf("failed to get filesystem stats: %w", err)
	}

	totalBytes := stat.Blocks * uint64(stat.Bsize)
	totalInodes := stat.Files
	freeInodes := stat.Ffree

	return &DiskStats{
		Filesystem: FilesystemStats{
			UsageBytes:  usageBytes,
			LimitBytes:  totalBytes,
			InodesFree:  freeInodes,
			InodesTotal: totalInodes,
		},
	}, nil
}
