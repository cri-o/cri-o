//go:build freebsd

package oci

import (
	"fmt"
	"syscall"

	"github.com/cri-o/cri-o/utils"
)

type DiskMetrics struct {
	Filesystem FilesystemMetrics
}

type FilesystemMetrics struct {
	UsageBytes  uint64 `json:"usage_bytes"`
	LimitBytes  uint64 `json:"limit_bytes"`
	InodesFree  uint64 `json:"inodes_free"`
	InodesTotal uint64 `json:"inodes_total"`
}

func GetDiskUsageForPath(path string) (*DiskMetrics, error) {
	usageBytes, _, err := utils.GetDiskUsageStats(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get disk usage stats: %w", err)
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return nil, fmt.Errorf("failed to get filesystem stats: %w", err)
	}

	totalBytes := stat.Blocks * stat.Bsize
	totalInodes := stat.Files
	freeInodes := uint64(stat.Ffree) // Ffree is int64 on FreeBSD

	return &DiskMetrics{
		Filesystem: FilesystemMetrics{
			UsageBytes:  usageBytes,
			LimitBytes:  totalBytes,
			InodesFree:  freeInodes,
			InodesTotal: totalInodes,
		},
	}, nil
}
