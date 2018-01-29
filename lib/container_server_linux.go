// +build linux

package lib

import (
	"path/filepath"
	"time"

	"github.com/kubernetes-incubator/cri-o/lib/sandbox"
	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/opencontainers/runc/libcontainer"
	selinux "github.com/opencontainers/selinux/go-selinux"
	"github.com/opencontainers/selinux/go-selinux/label"
)

// libcontainerStats gets the stats for the container with the given id from runc/libcontainer
func (c *ContainerServer) libcontainerStats(ctr *oci.Container) (*libcontainer.Stats, error) {
	// TODO: make this not hardcoded
	// was: c.runtime.Path(ociContainer) but that returns /usr/bin/runc - how do we get /run/runc?
	// runroot is /var/run/runc
	// Hardcoding probably breaks ClearContainers compatibility
	factory, err := loadFactory("/run/runc")
	if err != nil {
		return nil, err
	}
	container, err := factory.Load(ctr.ID())
	if err != nil {
		return nil, err
	}
	return container.Stats()
}

func (c *ContainerServer) addSandboxPlatform(sb *sandbox.Sandbox) {
	c.state.processLevels[selinux.NewContext(sb.ProcessLabel())["level"]]++
}

func (c *ContainerServer) removeSandboxPlatform(sb *sandbox.Sandbox) {
	processLabel := sb.ProcessLabel()
	level := selinux.NewContext(processLabel)["level"]
	pl, ok := c.state.processLevels[level]
	if ok {
		c.state.processLevels[level] = pl - 1
		if c.state.processLevels[level] == 0 {
			label.ReleaseLabel(processLabel)
			delete(c.state.processLevels, level)
		}
	}
}

func loadFactory(root string) (libcontainer.Factory, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	cgroupManager := libcontainer.Cgroupfs
	return libcontainer.New(abs, cgroupManager, libcontainer.CriuPath(""))
}

func (c *ContainerServer) getContainerStats(ctr *oci.Container, previousStats *ContainerStats) (*ContainerStats, error) {
	previousCPU := previousStats.CPUNano
	previousSystem := previousStats.SystemNano
	libcontainerStats, err := c.libcontainerStats(ctr)
	if err != nil {
		return nil, err
	}
	cgroupStats := libcontainerStats.CgroupStats
	stats := new(ContainerStats)
	stats.Container = ctr.ID()
	stats.CPUNano = cgroupStats.CpuStats.CpuUsage.TotalUsage
	stats.SystemNano = time.Now().UnixNano()
	stats.CPU = calculateCPUPercent(libcontainerStats, previousCPU, previousSystem)
	stats.MemUsage = cgroupStats.MemoryStats.Usage.Usage
	stats.MemLimit = getMemLimit(cgroupStats.MemoryStats.Usage.Limit)
	stats.MemPerc = float64(stats.MemUsage) / float64(stats.MemLimit)
	stats.PIDs = cgroupStats.PidsStats.Current
	stats.BlockInput, stats.BlockOutput = calculateBlockIO(libcontainerStats)
	stats.NetInput, stats.NetOutput = getContainerNetIO(libcontainerStats)

	return stats, nil
}
