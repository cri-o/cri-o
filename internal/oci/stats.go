package oci

import (
	"strings"
	"syscall"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containers/podman/v2/pkg/cgroups"
	"github.com/cri-o/ocicni/pkg/ocicni"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

// ContainerStats contains the statistics information for a running container
type ContainerStats struct {
	Container       string
	CPU             float64
	CPUNano         uint64
	SystemNano      int64
	MemUsage        uint64
	MemLimit        uint64
	MemPerc         float64
	NetInput        uint64
	NetOutput       uint64
	BlockInput      uint64
	BlockOutput     uint64
	PIDs            uint64
	WorkingSetBytes uint64
}

// Returns the total number of bytes transmitted and received for the given container stats
func getContainerNetIO(netNsPath string) (received, transmitted uint64) {
	ns.WithNetNSPath(netNsPath, func(_ ns.NetNS) error { // nolint: errcheck
		link, err := netlink.LinkByName(ocicni.DefaultInterfaceName)
		if err != nil {
			logrus.Warnf(
				"unable to retrieve network namespace link %s: %v",
				ocicni.DefaultInterfaceName, err,
			)
			return err
		}
		attrs := link.Attrs()
		if attrs != nil && attrs.Statistics != nil {
			received = attrs.Statistics.RxBytes
			transmitted = attrs.Statistics.TxBytes
		}
		return nil
	})

	return received, transmitted
}

func calculateBlockIO(stats *cgroups.Metrics) (read, write uint64) {
	for _, blkIOEntry := range stats.Blkio.IoServiceBytesRecursive {
		switch strings.ToLower(blkIOEntry.Op) {
		case "read":
			read += blkIOEntry.Value
		case "write":
			write += blkIOEntry.Value
		}
	}
	return read, write
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
