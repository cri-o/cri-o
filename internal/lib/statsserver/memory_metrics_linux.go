package statsserver

import (
	"github.com/opencontainers/cgroups"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/config/node"
	"github.com/cri-o/cri-o/internal/oci"
)

func generateContainerMemoryMetrics(ctr *oci.Container, mem *cgroups.MemoryStats) []*types.Metric {
	if mem == nil {
		return []*types.Metric{}
	}
	// Compute derived memory values
	workingSetBytes, rssBytes, pageFaults, majorPageFaults, _ := computeMemoryMetricValues(mem)
	swapUsage := computeSwapUsageForMetrics(mem)
	fileMapped := computeFileMapped(mem)

	memoryMetrics := []*containerMetric{
		{
			desc: containerMemoryCache,
			valueFunc: func() metricValues {
				return metricValues{{value: mem.Cache, metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: containerMemoryRss,
			valueFunc: func() metricValues {
				return metricValues{{value: rssBytes, metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: containerMemoryKernelUsage,
			valueFunc: func() metricValues {
				return metricValues{{value: mem.KernelUsage.Usage, metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: containerMemoryMappedFile,
			valueFunc: func() metricValues {
				return metricValues{{value: fileMapped, metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: containerMemorySwap,
			valueFunc: func() metricValues {
				return metricValues{{value: swapUsage, metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: containerMemoryFailcnt,
			valueFunc: func() metricValues {
				return metricValues{{value: mem.Usage.Failcnt, metricType: types.MetricType_COUNTER}}
			},
		},
		{
			desc: containerMemoryUsageBytes,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      mem.Usage.Usage,
					metricType: types.MetricType_GAUGE,
				}}
			},
		},
		{
			desc: containerMemoryMaxUsageBytes,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      mem.Usage.MaxUsage,
					metricType: types.MetricType_GAUGE,
				}}
			},
		},
		{
			desc: containerMemoryWorkingSetBytes,
			valueFunc: func() metricValues {
				return metricValues{{value: workingSetBytes, metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: containerMemoryFailuresTotal,
			valueFunc: func() metricValues {
				metrics := make([]metricValue, 0, 4)
				pgfaultMetrics := metricValues{
					{
						value:      pageFaults,
						labels:     []string{"pgfault", "container"},
						metricType: types.MetricType_COUNTER,
					},
					{
						value:      pageFaults,
						labels:     []string{"pgfault", "hierarchy"},
						metricType: types.MetricType_COUNTER,
					},
				}
				metrics = append(metrics, pgfaultMetrics...)
				pgmajfaultMetrics := metricValues{
					{
						value:      majorPageFaults,
						labels:     []string{"pgmajfault", "container"},
						metricType: types.MetricType_COUNTER,
					},
					{
						value:      majorPageFaults,
						labels:     []string{"pgmajfault", "hierarchy"},
						metricType: types.MetricType_COUNTER,
					},
				}
				metrics = append(metrics, pgmajfaultMetrics...)

				return metrics
			},
		},
	}

	return computeContainerMetrics(ctr, memoryMetrics)
}

// generateContainerMemoryExtraMetrics reports the memory metrics that have no
// cAdvisor equivalent, so they can be enabled independently of the cAdvisor
// compatible set in generateContainerMemoryMetrics.
func generateContainerMemoryExtraMetrics(ctr *oci.Container, mem *cgroups.MemoryStats) []*types.Metric {
	if mem == nil {
		return []*types.Metric{}
	}

	isCgroupV2 := node.CgroupIsV2()
	activeAnon, inactiveAnon := computeAnonMemory(mem, isCgroupV2)
	anonTHP, shmemTHP, fileTHP := computeTransparentHugepages(mem, isCgroupV2)

	memoryExtraMetrics := []*containerMetric{
		{
			desc: containerMemoryActiveAnonBytes,
			valueFunc: func() metricValues {
				return metricValues{{value: activeAnon, metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: containerMemoryInactiveAnonBytes,
			valueFunc: func() metricValues {
				return metricValues{{value: inactiveAnon, metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: containerMemoryAnonTHPBytes,
			valueFunc: func() metricValues {
				return metricValues{{value: anonTHP, metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: containerMemoryShmemTHPBytes,
			valueFunc: func() metricValues {
				return metricValues{{value: shmemTHP, metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: containerMemoryFileTHPBytes,
			valueFunc: func() metricValues {
				return metricValues{{value: fileTHP, metricType: types.MetricType_GAUGE}}
			},
		},
	}

	return computeContainerMetrics(ctr, memoryExtraMetrics)
}

// computeMemoryMetricValues computes derived memory statistics for metrics.
func computeMemoryMetricValues(memStats *cgroups.MemoryStats) (workingSetBytes, rssBytes, pageFaults, majorPageFaults, availableBytes uint64) {
	var inactiveFileName string

	usageBytes := memStats.Usage.Usage

	if node.CgroupIsV2() {
		rssBytes = memStats.Stats["anon"]
		inactiveFileName = "inactive_file"
		pageFaults = memStats.Stats["pgfault"]
		majorPageFaults = memStats.Stats["pgmajfault"]
	} else {
		inactiveFileName = "total_inactive_file"
		rssBytes = memStats.Stats["total_rss"]
	}

	workingSetBytes = usageBytes
	if v, ok := memStats.Stats[inactiveFileName]; ok {
		if workingSetBytes < v {
			workingSetBytes = 0
		} else {
			workingSetBytes -= v
		}
	}

	return workingSetBytes, rssBytes, pageFaults, majorPageFaults, availableBytes
}

// computeSwapUsageForMetrics computes the actual swap usage for metrics.
func computeSwapUsageForMetrics(memStats *cgroups.MemoryStats) uint64 {
	if node.CgroupIsV2() {
		if memStats.SwapUsage.Usage > memStats.Usage.Usage {
			return memStats.SwapUsage.Usage - memStats.Usage.Usage
		}

		return 0
	}

	return memStats.SwapUsage.Usage
}

// computeFileMapped computes the file mapped memory value.
func computeFileMapped(memStats *cgroups.MemoryStats) uint64 {
	if node.CgroupIsV2() {
		return memStats.Stats["file_mapped"]
	}

	if memStats.UseHierarchy {
		return memStats.Stats["total_mapped_file"]
	}

	return memStats.Stats["mapped_file"]
}

// computeAnonMemory computes the active and inactive anonymous memory values.
// Both cgroup versions report these, but v1 prefixes the hierarchical totals
// with "total_". The cgroup version is passed in so both branches are
// exercisable regardless of the host.
func computeAnonMemory(memStats *cgroups.MemoryStats, isCgroupV2 bool) (activeAnon, inactiveAnon uint64) {
	if isCgroupV2 {
		return memStats.Stats["active_anon"], memStats.Stats["inactive_anon"]
	}

	return memStats.Stats["total_active_anon"], memStats.Stats["total_inactive_anon"]
}

// computeTransparentHugepages computes the amount of memory backed by
// transparent hugepages, split by the kind of memory backing it. These keys
// only exist under cgroup v2; cgroup v1 has no equivalent, so all three are
// reported as zero there. The cgroup version is passed in so both branches are
// exercisable regardless of the host.
func computeTransparentHugepages(memStats *cgroups.MemoryStats, isCgroupV2 bool) (anonTHP, shmemTHP, fileTHP uint64) {
	if !isCgroupV2 {
		return 0, 0, 0
	}

	return memStats.Stats["anon_thp"], memStats.Stats["shmem_thp"], memStats.Stats["file_thp"]
}

func GenerateContainerOOMMetrics(ctr *oci.Container, oomCount uint64) []*types.Metric {
	oomMetrics := []*containerMetric{
		{
			desc: containerOomEventsTotal,
			valueFunc: func() metricValues {
				return metricValues{{value: oomCount, metricType: types.MetricType_COUNTER}}
			},
		},
	}

	return computeContainerMetrics(ctr, oomMetrics)
}
