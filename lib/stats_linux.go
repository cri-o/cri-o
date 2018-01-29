package lib

import (
	"strings"
	"syscall"
	"time"

	"github.com/opencontainers/runc/libcontainer"
)

// Returns the total number of bytes transmitted and received for the given container stats
func getContainerNetIO(stats *libcontainer.Stats) (received uint64, transmitted uint64) {
	for _, iface := range stats.Interfaces {
		received += iface.RxBytes
		transmitted += iface.TxBytes
	}
	return
}

func calculateCPUPercent(stats *libcontainer.Stats, previousCPU uint64, previousSystem int64) float64 {
	var (
		cpuPercent  = 0.0
		cpuDelta    = float64(stats.CgroupStats.CpuStats.CpuUsage.TotalUsage - previousCPU)
		systemDelta = float64(uint64(time.Now().UnixNano()) - uint64(previousSystem))
	)
	if systemDelta > 0.0 && cpuDelta > 0.0 {
		// gets a ratio of container cpu usage total, multiplies it by the number of cores (4 cores running
		// at 100% utilization should be 400% utilization), and multiplies that by 100 to get a percentage
		cpuPercent = (cpuDelta / systemDelta) * float64(len(stats.CgroupStats.CpuStats.CpuUsage.PercpuUsage)) * 100
	}
	return cpuPercent
}

func calculateBlockIO(stats *libcontainer.Stats) (read uint64, write uint64) {
	for _, blkIOEntry := range stats.CgroupStats.BlkioStats.IoServiceBytesRecursive {
		switch strings.ToLower(blkIOEntry.Op) {
		case "read":
			read += blkIOEntry.Value
		case "write":
			write += blkIOEntry.Value
		}
	}
	return
}

// getMemory limit returns the memory limit for a given cgroup
// If the configured memory limit is larger than the total memory on the sys, the
// physical system memory size is returned
func getMemLimit(cgroupLimit uint64) uint64 {
	si := &syscall.Sysinfo_t{}
	err := syscall.Sysinfo(si)
	if err != nil {
		return cgroupLimit
	}

	physicalLimit := uint64(si.Totalram)
	if cgroupLimit > physicalLimit {
		return physicalLimit
	}
	return cgroupLimit
}
