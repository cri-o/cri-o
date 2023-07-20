package cgmgr

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/containers/common/pkg/cgroups"
	"github.com/cri-o/cri-o/internal/config/node"
	libctrcgroups "github.com/opencontainers/runc/libcontainer/cgroups"
	"github.com/sirupsen/logrus"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func populateSandboxCgroupStatsFromPath(cgroupPath string, stats *types.PodSandboxStats) error {
	cgroupStats, err := cgroupStatsFromPath(cgroupPath)
	if err != nil {
		return err
	}
	systemNano := time.Now().UnixNano()
	stats.Linux.Cpu = createCPUStats(systemNano, cgroupStats)
	stats.Linux.Process = createProcessUsage(systemNano, cgroupStats)
	stats.Linux.Memory, err = createMemoryStats(systemNano, cgroupStats, cgroupPath)
	return err
}

func populateContainerCgroupStatsFromPath(cgroupPath string, stats *types.ContainerStats) error {
	// checks cgroup just for the container, not the entire pod
	cgroupStats, err := cgroupStatsFromPath(cgroupPath)
	if err != nil {
		return err
	}
	systemNano := time.Now().UnixNano()
	stats.Cpu = createCPUStats(systemNano, cgroupStats)
	stats.Memory, err = createMemoryStats(systemNano, cgroupStats, cgroupPath)
	return err
}

func cgroupStatsFromPath(cgroupPath string) (*libctrcgroups.Stats, error) {
	cg, err := cgroups.Load(cgroupPath)
	if err != nil {
		return nil, fmt.Errorf("unable to load cgroup at %s: %w", cgroupPath, err)
	}

	return cg.Stat()
}

func createCPUStats(systemNano int64, cgroupStats *libctrcgroups.Stats) *types.CpuUsage {
	return &types.CpuUsage{
		Timestamp:            systemNano,
		UsageCoreNanoSeconds: &types.UInt64Value{Value: cgroupStats.CpuStats.CpuUsage.TotalUsage},
	}
}

func createMemoryStats(systemNano int64, cgroupStats *libctrcgroups.Stats, cgroupPath string) (*types.MemoryUsage, error) {
	memUsage := cgroupStats.MemoryStats.Usage.Usage
	memLimit := MemLimitGivenSystem(cgroupStats.MemoryStats.Usage.Limit)

	memory := &types.MemoryUsage{
		Timestamp:       systemNano,
		WorkingSetBytes: &types.UInt64Value{},
		RssBytes:        &types.UInt64Value{},
		PageFaults:      &types.UInt64Value{},
		MajorPageFaults: &types.UInt64Value{},
		UsageBytes:      &types.UInt64Value{Value: memUsage},
		AvailableBytes:  &types.UInt64Value{Value: memUsage - memLimit},
	}

	if err := updateWithMemoryStats(cgroupPath, memory, memUsage); err != nil {
		return memory, fmt.Errorf("unable to update with memory.stat info: %w", err)
	}
	return memory, nil
}

// MemLimitGivenSystem limit returns the memory limit for a given cgroup
// If the configured memory limit is larger than the total memory on the sys, the
// physical system memory size is returned
func MemLimitGivenSystem(cgroupLimit uint64) uint64 {
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

// updateWithMemoryStats updates the ContainerStats object with info
// from cgroup's memory.stat. Returns an error if the file does not exists,
// or not parsable.
func updateWithMemoryStats(path string, memory *types.MemoryUsage, usage uint64) error {
	const memoryStatFile = "memory.stat"
	var memoryStatPath, inactiveFileSearchString string
	if !node.CgroupIsV2() {
		memoryStatPath = filepath.Join(cgroupMemoryPathV1, path, memoryStatFile)
		inactiveFileSearchString = "total_inactive_file "
	} else {
		memoryStatPath = filepath.Join(cgroupMemoryPathV2, path, memoryStatFile)
		inactiveFileSearchString = "inactive_file "
	}
	return UpdateWithMemoryStatsFromFile(memoryStatPath, inactiveFileSearchString, memory, usage)
}

func UpdateWithMemoryStatsFromFile(memoryStatPath, inactiveFileSearchString string, memory *types.MemoryUsage, usage uint64) error {
	var totalInactive uint64
	f, err := os.Open(memoryStatPath)
	if err != nil {
		return err
	}
	defer f.Close()

	toUpdate := []struct {
		prefix string
		field  *uint64
	}{
		{inactiveFileSearchString, &totalInactive},
		{"rss ", &memory.RssBytes.Value},
		{"pgfault ", &memory.PageFaults.Value},
		{"pgmajfault ", &memory.MajorPageFaults.Value},
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		for _, field := range toUpdate {
			if !strings.HasPrefix(scanner.Text(), field.prefix) {
				continue
			}
			val, err := strconv.ParseUint(
				strings.TrimPrefix(scanner.Text(), field.prefix), 10, 64,
			)
			if err != nil {
				return fmt.Errorf("unable to parse %s: %w", field.prefix, err)
			}
			*field.field = val
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if usage > totalInactive {
		memory.WorkingSetBytes.Value = usage - totalInactive
	} else {
		logrus.Warnf(
			"Unable to account working set stats: total_inactive_file (%d) > memory usage (%d)",
			totalInactive, usage,
		)
	}

	return nil
}

func createProcessUsage(systemNano int64, cgroupStats *libctrcgroups.Stats) *types.ProcessUsage {
	return &types.ProcessUsage{
		Timestamp:    systemNano,
		ProcessCount: &types.UInt64Value{Value: cgroupStats.PidsStats.Current},
	}
}
