//go:build !linux
// +build !linux

package cgmgr

import (
	"github.com/containers/storage/pkg/system"
)

// MemLimitGivenSystem limit returns the memory limit for a given cgroup
// If the configured memory limit is larger than the total memory on the sys, the
// physical system memory size is returned
func MemLimitGivenSystem(cgroupLimit uint64) uint64 {
	meminfo, err := system.ReadMemInfo()
	if err != nil {
		return cgroupLimit
	}
	return uint64(meminfo.MemTotal)
}
