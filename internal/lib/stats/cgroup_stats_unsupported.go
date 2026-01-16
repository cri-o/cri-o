//go:build !linux

package stats

// CgroupStats contains cgroup statistics for unsupported platforms.
// This is supposed to be empty.
type CgroupStats struct{}
