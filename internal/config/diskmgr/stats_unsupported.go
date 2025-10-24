//go:build !linux

package diskmgr

// DiskMetrics represents disk usage statistics
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

// Unsupported platform implementations
func GetDiskUsageForPath(path string) (*DiskMetrics, error) {
	return &DiskMetrics{}, nil
}
