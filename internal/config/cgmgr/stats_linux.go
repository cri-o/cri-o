package cgmgr

import (
	"context"
	"math"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/opencontainers/cgroups"
	"github.com/opencontainers/cgroups/manager"

	"github.com/cri-o/cri-o/internal/config/node"
	"github.com/cri-o/cri-o/internal/log"
)

// This is a universal stats object to be used across different runtime implementations.
// We could have used the libcontainer/cgroups.Stats object as a standard stats object for cri-o.
// But due to it's incompatibility with non-linux platforms,
// we have to create our own object that can be moved around regardless of the runtime.
type CgroupStats struct {
	Memory     *MemoryStats
	CPU        *CPUStats
	Hugetlb    map[string]HugetlbStats
	Pid        *PidsStats
	DiskIO     *DiskIOStats
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
	// Amount of cached filesystem data mapped with mmap().
	FileMapped uint64
	// The number of memory usage hits limits. For cgroup v1 only.
	Failcnt uint64
}

type CPUStats struct {
	TotalUsageNano uint64
	PerCPUUsage    []uint64
	// Time spent by tasks of the cgroup in kernel mode in nanoseconds.
	UsageInKernelmode uint64
	// Time spent by tasks of the cgroup in user mode in nanoseconds.
	UsageInUsermode uint64
	// Number of periods with throttling active
	ThrottlingActivePeriods uint64
	// Number of periods when the container hit its throttling limit.
	ThrottledPeriods uint64
	// Aggregate time the container was throttled for in nanoseconds.
	ThrottledTime uint64
}

type HugetlbStats struct {
	Usage uint64
	Max   uint64
}

type PidsStats struct {
	Current         uint64
	Limit           uint64
	Pids            []int
	FileDescriptors uint64
	Sockets         uint64
	Threads         uint64
	ThreadsMax      uint64
	UlimitsSoft     uint64
}

type DiskIOStats struct {
	IoServiceBytes []cgroups.BlkioStatEntry
	IoServiced     []cgroups.BlkioStatEntry
}

// MemLimitGivenSystem limit returns the memory limit for a given cgroup
// If the configured memory limit is larger than the total memory on the sys, the
// physical system memory size is returned.
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

func libctrManager(cgroup, parent string, systemd bool) (cgroups.Manager, error) {
	if systemd {
		parent = filepath.Base(parent)
		if parent == "." {
			// libcontainer shorthand for root
			// see https://github.com/opencontainers/runc/blob/9fffadae8/libcontainer/cgroups/systemd/common.go#L71
			parent = "-.slice"
		}
	}

	cg := &cgroups.Cgroup{
		Name:   cgroup,
		Parent: parent,
		Resources: &cgroups.Resources{
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

func statsFromLibctrMgr(cgMgr cgroups.Manager) (*CgroupStats, error) {
	stats, err := cgMgr.GetStats()
	if err != nil {
		return nil, err
	}

	pids, err := cgMgr.GetPids()
	if err != nil {
		return nil, err
	}

	return &CgroupStats{
		Memory:  cgroupMemStats(&stats.MemoryStats),
		CPU:     cgroupCPUStats(&stats.CpuStats),
		Hugetlb: cgroupHugetlbStats(stats.HugetlbStats),
		Pid:     cgroupPidStats(stats, pids),
		DiskIO: &DiskIOStats{
			IoServiced:     stats.BlkioStats.IoServicedRecursive,
			IoServiceBytes: stats.BlkioStats.IoServiceBytesRecursive,
		},
		SystemNano: time.Now().UnixNano(),
	}, nil
}

func cgroupMemStats(memStats *cgroups.MemoryStats) *MemoryStats {
	var (
		workingSetBytes  uint64
		rssBytes         uint64
		pageFaults       uint64
		majorPageFaults  uint64
		usageBytes       uint64
		availableBytes   uint64
		inactiveFileName string
		memSwap          uint64
		fileMapped       uint64
		failcnt          uint64
	)

	usageBytes = memStats.Usage.Usage

	if node.CgroupIsV2() {
		// Use anon for rssBytes for cgroup v2 as in cAdvisor
		// See: https://github.com/google/cadvisor/blob/786dbcfdf5b1aae8341b47e71ab115066a9b4c06/container/libcontainer/handler.go#L809
		rssBytes = memStats.Stats["anon"]
		inactiveFileName = "inactive_file"
		pageFaults = memStats.Stats["pgfault"]
		majorPageFaults = memStats.Stats["pgmajfault"]
		fileMapped = memStats.Stats["file_mapped"]
		// libcontainer adds memory.swap.current to memory.current and reports them as SwapUsage to be compatible with cgroup v1,
		// because cgroup v1 reports SwapUsage as mem+swap combined.
		// Here we subtract SwapUsage from memory usage to get the actual swap value.
		memSwap = memStats.SwapUsage.Usage - usageBytes
	} else {
		inactiveFileName = "total_inactive_file"
		rssBytes = memStats.Stats["total_rss"]
		memSwap = memStats.SwapUsage.Usage

		fileMapped = memStats.Stats["mapped_file"]
		if memStats.UseHierarchy {
			fileMapped = memStats.Stats["total_mapped_file"]
		}

		// cgroup v1 doesn't have equivalent stats for pgfault and pgmajfault
		failcnt = memStats.Usage.Failcnt
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
		Usage:           usageBytes,
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
		SwapUsage:       memSwap,
		SwapLimit:       memStats.SwapUsage.Limit,
		FileMapped:      fileMapped,
		Failcnt:         failcnt,
	}
}

func cgroupCPUStats(cpuStats *cgroups.CpuStats) *CPUStats {
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

func cgroupHugetlbStats(cgHugetlbStats map[string]cgroups.HugetlbStats) map[string]HugetlbStats {
	hugetlbStats := map[string]HugetlbStats{}

	for pagesize, hugetlb := range cgHugetlbStats {
		hugetlbStats[pagesize] = HugetlbStats{
			Usage: hugetlb.Usage,
			Max:   hugetlb.MaxUsage,
		}
	}

	return hugetlbStats
}

func isMemoryUnlimited(v uint64) bool {
	// if the container has unlimited memory, the value of memory.max (in cgroupv2) will be "max"
	// or the value of memory.limit_in_bytes (in cgroupv1) will be -1
	// either way, libcontainer/cgroups will return math.MaxUint64
	return v == math.MaxUint64
}

func cgroupPidStats(stats *cgroups.Stats, pids []int) *PidsStats {
	var fdCount, socketCount, ulimitsSoft uint64

	// This is based on the cadvisor handler: https://github.com/google/cadvisor/blob/master/container/libcontainer/handler.go
	for _, pid := range pids {
		addFdsForProcess(pid, &fdCount, &socketCount)
		addUlimitsForProcess(pid, &ulimitsSoft)
	}

	return &PidsStats{
		Current:         stats.PidsStats.Current,
		Limit:           stats.PidsStats.Limit,
		Pids:            pids,
		FileDescriptors: fdCount,
		Sockets:         socketCount,
		Threads:         stats.PidsStats.Current,
		ThreadsMax:      stats.PidsStats.Limit,
		UlimitsSoft:     ulimitsSoft,
	}
}

func addFdsForProcess(pid int, fdCount, socketCount *uint64) {
	if fdCount == nil || socketCount == nil {
		panic("Programming error: fdCount or socketCount should not be nil")
	}

	dirPath := path.Join("/proc", strconv.Itoa(pid), "fd")

	fds, err := os.ReadDir(dirPath)
	if err != nil {
		log.Infof(context.Background(), "error while listing directory %q to measure fd count: %v", dirPath, err)

		return
	}

	*fdCount += uint64(len(fds))
	for _, fd := range fds {
		fdPath := path.Join(dirPath, fd.Name())

		linkName, err := os.Readlink(fdPath)
		if err != nil {
			log.Infof(context.Background(), "error while reading %q link: %v", fdPath, err)

			continue
		}

		if strings.HasPrefix(linkName, "socket") {
			*socketCount++
		}
	}
}

func addUlimitsForProcess(pid int, limits *uint64) {
	if limits == nil {
		panic("Programming error: limits should not be nil")
	}

	limitsPath := path.Join("/proc", strconv.Itoa(pid), "limits")

	limitsData, err := os.ReadFile(limitsPath)
	if err != nil {
		log.Infof(context.Background(), "error while reading %q to get thread limits: %v", limitsPath, err)

		return
	}

	for line := range strings.SplitSeq(string(limitsData), "\n") {
		if !strings.HasPrefix(line, "Max open files") {
			continue
		}

		const maxOpenFilesPrefix = "Max open files"

		remainingLine := strings.TrimSpace(line[len(maxOpenFilesPrefix):])

		fields := strings.Fields(remainingLine)
		if len(fields) >= 1 {
			if softLimit, err := strconv.ParseUint(fields[0], 10, 64); err == nil {
				*limits = softLimit
			}
		}

		return
	}
}
