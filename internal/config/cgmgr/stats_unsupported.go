//go:build !linux
// +build !linux

package cgmgr

import (
	"github.com/containers/storage/pkg/system"
)

type CgroupStats struct {
	Memory     *MemoryStats
	CPU        *CPUStats
	Pid        *PidsStats
	SystemNano int64
}

type MemoryStats struct {
	Usage           uint64
	Cache           uint64
	Limit           uint64
	MaxUsage        uint64
	WorkingSetBytes uint64
	RssBytes        uint64
	PageFaults      uint64
	MajorPageFaults uint64
	AvailableBytes  uint64
	KernelUsage     uint64
	KernelTCPUsage  uint64
	SwapUsage       uint64
	SwapLimit       uint64
	FileMapped      uint64
	Failcnt         uint64
}

type CPUStats struct {
	TotalUsageNano    uint64
	PerCPUUsage       []uint64
	UsageInKernelmode uint64
	UsageInUsermode   uint64
	// Number of periods with throttling active
	ThrottlingActivePeriods uint64
	// Number of periods when the container hit its throttling limit.
	ThrottledPeriods uint64
	// Aggregate time the container was throttled for in nanoseconds.
	ThrottledTime uint64
}

type PidsStats struct {
	Current uint64
	Limit   uint64
}

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
