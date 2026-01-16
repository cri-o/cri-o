//go:build !linux && !freebsd

package stats

import "errors"

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

// GetDiskUsageForPath is not supported on this platform.
func GetDiskUsageForPath(path string) (*DiskStats, error) {
	return nil, errors.New("disk usage statistics not supported on this platform")
}
