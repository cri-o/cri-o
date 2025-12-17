package statsserver

import (
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/config/cgmgr"
	"github.com/cri-o/cri-o/internal/oci"
)

func generateContainerMemoryMetrics(ctr *oci.Container, mem *cgmgr.MemoryStats) []*types.Metric {
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
				return metricValues{{value: mem.RssBytes, metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: containerMemoryKernelUsage,
			valueFunc: func() metricValues {
				return metricValues{{value: mem.KernelUsage, metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: containerMemoryMappedFile,
			valueFunc: func() metricValues {
				return metricValues{{value: mem.FileMapped, metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: containerMemorySwap,
			valueFunc: func() metricValues {
				return metricValues{{value: mem.SwapUsage, metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: containerMemoryFailcnt,
			valueFunc: func() metricValues {
				return metricValues{{value: mem.Failcnt, metricType: types.MetricType_COUNTER}}
			},
		},
		{
			desc: containerMemoryUsageBytes,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      mem.Usage,
					metricType: types.MetricType_GAUGE,
				}}
			},
		},
		{
			desc: containerMemoryMaxUsageBytes,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      mem.MaxUsage,
					metricType: types.MetricType_GAUGE,
				}}
			},
		},
		{
			desc: containerMemoryWorkingSetBytes,
			valueFunc: func() metricValues {
				return metricValues{{value: mem.WorkingSetBytes, metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: containerMemoryFailuresTotal,
			valueFunc: func() metricValues {
				metrics := make([]metricValue, 0)
				pgfaultMetrics := metricValues{
					{
						value:      mem.PageFaults,
						labels:     []string{"pgfault", "container"},
						metricType: types.MetricType_COUNTER,
					},
					{
						value:      mem.PageFaults,
						labels:     []string{"pgfault", "hierarchy"},
						metricType: types.MetricType_COUNTER,
					},
				}
				metrics = append(metrics, pgfaultMetrics...)
				pgmajfaultMetrics := metricValues{
					{
						value:      mem.MajorPageFaults,
						labels:     []string{"pgmajfault", "container"},
						metricType: types.MetricType_COUNTER,
					},
					{
						value:      mem.MajorPageFaults,
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
