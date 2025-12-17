//go:build !linux && !freebsd

package stats

import "fmt"

// DiskStats represents comprehensive disk statistics for a container
type DiskMetrics struct {
	Filesystem FilesystemMetrics
}

// FilesystemStats represents filesystem usage statistics
type FilesystemMetrics struct {
	UsageBytes  uint64 `json:"usage_bytes"`
	LimitBytes  uint64 `json:"limit_bytes"`
	InodesFree  uint64 `json:"inodes_free"`
	InodesTotal uint64 `json:"inodes_total"`
}

func GetDiskUsageForPath(path string) (*DiskMetrics, error) {
	return nil, fmt.Errorf("disk usage statistics not supported on this platform")
}
