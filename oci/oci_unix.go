// +build !windows

package oci

import (
	"fmt"
	"os/exec"
	"time"

	"github.com/opencontainers/runc/libcontainer"
	"golang.org/x/sys/unix"
)

const (
	// ContainerExitsDir is the location of container exit dirs
	ContainerExitsDir = "/var/run/crio/exits"
	// ContainerAttachSocketDir is the location for container attach sockets
	ContainerAttachSocketDir = "/var/run/crio"
)

func kill(pid int) error {
	err := unix.Kill(pid, unix.SIGKILL)
	if err != nil && err != unix.ESRCH {
		return fmt.Errorf("failed to kill process: %v", err)
	}
	return nil
}

func getExitCode(err error) int32 {
	if exitErr, ok := err.(*exec.ExitError); ok {
		if status, ok := exitErr.Sys().(unix.WaitStatus); ok {
			return int32(status.ExitStatus())
		}
	}
	return -1
}

func calculateCPUPercent(stats *libcontainer.Stats) float64 {
	var (
		cpuPercent = 0.0
		cpuUsage   = float64(stats.CgroupStats.CpuStats.CpuUsage.TotalUsage)
		systemTime = float64(uint64(time.Now().UnixNano()))
	)
	if systemTime > 0.0 && cpuUsage > 0.0 {
		// gets a ratio of container cpu usage total, multiplies it by the number of cores (4 cores running
		// at 100% utilization should be 400% utilization), and multiplies that by 100 to get a percentage
		cpuPercent = (cpuUsage / systemTime) * float64(len(stats.CgroupStats.CpuStats.CpuUsage.PercpuUsage)) * 100
	}
	return cpuPercent
}
