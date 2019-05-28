package oci

import (
	"strings"
	"syscall"

	"github.com/opencontainers/runc/libcontainer"
)

// ContainerStats contains the statistics information for a running container
type ContainerStats struct {
	Container   string
	CPU         float64
	CPUNano     uint64
	SystemNano  int64
	MemUsage    uint64
	MemLimit    uint64
	MemPerc     float64
	NetInput    uint64
	NetOutput   uint64
	BlockInput  uint64
	BlockOutput uint64
	PIDs        uint64
}

// Returns the total number of bytes transmitted and received for the given container stats
func getContainerNetIO(stats *libcontainer.Stats) (received uint64, transmitted uint64) {
	for _, iface := range stats.Interfaces {
		received += iface.RxBytes
		transmitted += iface.TxBytes
	}
	return
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

	// conversion to uint64 needed to build on 32-bit
	// but lint complains about unnecessary conversion
	// see: pr#2409
	physicalLimit := uint64(si.Totalram) //nolint:unconvert
	if cgroupLimit > physicalLimit {
		return physicalLimit
	}
	return cgroupLimit
}
