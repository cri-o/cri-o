//go:build !linux

package utils

import "fmt"

// GetRealPhysicalUsage is not supported on non-Linux platforms.
func GetRealPhysicalUsage(path string) (uniqueBytes, inodeCount uint64, err error) {
	return 0, 0, fmt.Errorf("GetRealPhysicalUsage is only supported on Linux")
}
