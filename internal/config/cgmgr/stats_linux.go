package cgmgr

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/containers/common/pkg/cgroups"
	"github.com/cri-o/cri-o/internal/config/node"
	libctrcgroups "github.com/opencontainers/runc/libcontainer/cgroups"
	"github.com/opencontainers/runc/libcontainer/cgroups/manager"
	cgcfgs "github.com/opencontainers/runc/libcontainer/configs"
	"github.com/sirupsen/logrus"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// This is a universal stats object to be used across different runtime implementations.
// We could have used the libcontainer/cgroups.Stats object as a standard stats object for cri-o.
// But due to it's incompatibility with non-linux platforms,
// we have to create our own object that can be moved around regardless of the runtime.
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

func libctrManager(cgroup, parent string, systemd bool) (libctrcgroups.Manager, error) {
	if systemd {
		parent = filepath.Base(parent)
		if parent == "." {
			// libcontainer shorthand for root
			// see https://github.com/opencontainers/runc/blob/9fffadae8/libcontainer/cgroups/systemd/common.go#L71
			parent = "-.slice"
		}
	}
	cg := &cgcfgs.Cgroup{
		Name:   cgroup,
		Parent: parent,
		Resources: &cgcfgs.Resources{
			SkipDevices: true,
		},
		Systemd: systemd,
		// If the cgroup manager is systemd, then libcontainer
		// will construct the cgroup path (for scopes) as:
		// ScopePrefix-Name.scope. For slices, and for cgroupfs manager,
		// this will be ignored.
		// See: https://github.com/opencontainers/runc/tree/main/libcontainer/cgroups/systemd/common.go:getUnitName
		ScopePrefix: CrioPrefix,
	}
	return manager.New(cg)
}

func libctrStatsToCgroupStats(stats *libctrcgroups.Stats) *CgroupStats {
	return &CgroupStats{
		Memory: cgroupMemStats(&stats.MemoryStats),
		CPU:    cgroupCPUStats(&stats.CpuStats),
		Pid: &PidsStats{
			Current: stats.PidsStats.Current,
			Limit:   stats.PidsStats.Limit,
		},
		SystemNano: time.Now().UnixNano(),
	}
}

func cgroupMemStats(memStats *libctrcgroups.MemoryStats) *MemoryStats {
	var (
		workingSetBytes  uint64
		rssBytes         uint64
		pageFaults       uint64
		majorPageFaults  uint64
		usageBytes       uint64
		availableBytes   uint64
		inactiveFileName string
	)
	usageBytes = memStats.Usage.Usage
	if node.CgroupIsV2() {
		// Use anon for rssBytes for cgroup v2 as in cAdvisor
		// See: https://github.com/google/cadvisor/blob/786dbcfdf5b1aae8341b47e71ab115066a9b4c06/container/libcontainer/handler.go#L809
		rssBytes = memStats.Stats["anon"]
		inactiveFileName = "inactive_file"
		pageFaults = memStats.Stats["pgfault"]
		majorPageFaults = memStats.Stats["pgmajfault"]
	} else {
		inactiveFileName = "total_inactive_file"
		rssBytes = memStats.Stats["total_rss"]
		// cgroup v1 doesn't have equivalent stats for pgfault and pgmajfault
	}
	workingSetBytes = usageBytes
	if v, ok := memStats.Stats[inactiveFileName]; ok {
		if workingSetBytes < v {
			workingSetBytes = 0
		} else {
			workingSetBytes -= v
		}
	}
	if !isMemoryUnlimited(memStats.Usage.Limit) {
		// https://github.com/kubernetes/kubernetes/blob/94f15bbbcbe952762b7f5e6e3f77d86ecec7d7c2/pkg/kubelet/stats/helper.go#L69
		availableBytes = memStats.Usage.Limit - workingSetBytes
	}
	return &MemoryStats{
		Usage:           memStats.Usage.Usage,
		Cache:           memStats.Cache,
		Limit:           memStats.Usage.Limit,
		MaxUsage:        memStats.Usage.MaxUsage,
		WorkingSetBytes: workingSetBytes,
		RssBytes:        rssBytes,
		PageFaults:      pageFaults,
		MajorPageFaults: majorPageFaults,
		AvailableBytes:  availableBytes,
		KernelUsage:     memStats.KernelUsage.Usage,
		KernelTCPUsage:  memStats.KernelTCPUsage.Usage,
		SwapUsage:       memStats.SwapUsage.Usage,
		SwapLimit:       memStats.SwapUsage.Limit,
	}
}

func cgroupCPUStats(cpuStats *libctrcgroups.CpuStats) *CPUStats {
	return &CPUStats{
		TotalUsageNano:          cpuStats.CpuUsage.TotalUsage,
		PerCPUUsage:             cpuStats.CpuUsage.PercpuUsage,
		UsageInKernelmode:       cpuStats.CpuUsage.UsageInKernelmode,
		UsageInUsermode:         cpuStats.CpuUsage.UsageInUsermode,
		ThrottlingActivePeriods: cpuStats.ThrottlingData.Periods,
		ThrottledPeriods:        cpuStats.ThrottlingData.ThrottledPeriods,
		ThrottledTime:           cpuStats.ThrottlingData.ThrottledTime,
	}
}

func isMemoryUnlimited(v uint64) bool {
	// if the container has unlimited memory, the value of memory.max (in cgroupv2) will be "max"
	// or the value of memory.limit_in_bytes (in cgroupv1) will be -1
	// either way, libcontainer/cgroups will return math.MaxUint64
	return v == math.MaxUint64
}
