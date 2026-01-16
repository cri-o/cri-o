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
				metrics := make([]metricValue, 0)
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

	return computeContainerMetrics(ctr, memoryMetrics, "memory")
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

func GenerateContainerOOMMetrics(ctr *oci.Container, oomCount uint64) []*types.Metric {
	oomMetrics := []*containerMetric{
		{
			desc: containerOomEventsTotal,
			valueFunc: func() metricValues {
				return metricValues{{value: oomCount, metricType: types.MetricType_COUNTER}}
			},
		},
	}

	return computeContainerMetrics(ctr, oomMetrics, "oom")
}
