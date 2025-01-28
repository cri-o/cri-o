package statsserver

import (
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/config/cgmgr"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/oci"
)

func generateSandboxMemoryMetrics(sb *sandbox.Sandbox, mem *cgmgr.MemoryStats) []*types.Metric {
	memoryMetrics := []*containerMetric{
		{
			desc: &types.MetricDescriptor{
				Name:      "container_memory_cache",
				Help:      "Number of bytes of page cache memory.",
				LabelKeys: baseLabelKeys,
			},
			valueFunc: func() metricValues {
				return metricValues{{value: mem.Cache, metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: &types.MetricDescriptor{
				Name:      "container_memory_rss",
				Help:      "Size of RSS in bytes.",
				LabelKeys: baseLabelKeys,
			},
			valueFunc: func() metricValues {
				return metricValues{{value: mem.RssBytes, metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: &types.MetricDescriptor{
				Name:      "container_memory_kernel_usage",
				Help:      "Size of kernel memory allocated in bytes.",
				LabelKeys: baseLabelKeys,
			},
			valueFunc: func() metricValues {
				return metricValues{{value: mem.KernelUsage, metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: &types.MetricDescriptor{
				Name:      "container_memory_mapped_file",
				Help:      "Size of memory mapped files in bytes.",
				LabelKeys: baseLabelKeys,
			},
			valueFunc: func() metricValues {
				return metricValues{{value: mem.FileMapped, metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: &types.MetricDescriptor{
				Name:      "container_memory_swap",
				Help:      "Container swap usage in bytes.",
				LabelKeys: baseLabelKeys,
			},
			valueFunc: func() metricValues {
				return metricValues{{value: mem.SwapUsage, metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: &types.MetricDescriptor{
				Name:      "container_memory_failcnt",
				Help:      "Number of memory usage hits limits",
				LabelKeys: baseLabelKeys,
			},
			valueFunc: func() metricValues {
				return metricValues{{value: mem.Failcnt, metricType: types.MetricType_COUNTER}}
			},
		},
		{
			desc: &types.MetricDescriptor{
				Name:      "container_memory_usage_bytes",
				Help:      "Current memory usage in bytes, including all memory regardless of when it was accessed",
				LabelKeys: baseLabelKeys,
			},
			valueFunc: func() metricValues {
				return metricValues{{
					value:      mem.Usage,
					metricType: types.MetricType_GAUGE,
				}}
			},
		},
		{
			desc: &types.MetricDescriptor{
				Name:      "container_memory_max_usage_bytes",
				Help:      "Maximum memory usage recorded in bytes",
				LabelKeys: baseLabelKeys,
			},
			valueFunc: func() metricValues {
				return metricValues{{
					value:      mem.MaxUsage,
					metricType: types.MetricType_GAUGE,
				}}
			},
		},
		{
			desc: &types.MetricDescriptor{
				Name:      "container_memory_working_set_bytes",
				Help:      "Current working set in bytes.",
				LabelKeys: baseLabelKeys,
			},
			valueFunc: func() metricValues {
				return metricValues{{value: mem.WorkingSetBytes, metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: &types.MetricDescriptor{
				Name:      "container_memory_failures_total",
				Help:      "Cumulative count of memory allocation failures.",
				LabelKeys: append(baseLabelKeys, "failure_type", "scope"),
			},
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

	return computeSandboxMetrics(sb, memoryMetrics, "memory")
}

func GenerateSandboxOOMMetrics(sb *sandbox.Sandbox, c *oci.Container, oomCount uint64) []*types.Metric {
	oomMetrics := []*containerMetric{
		{
			desc: &types.MetricDescriptor{
				Name:      "container_oom_events_total",
				Help:      "Count of out of memory events observed for the container",
				LabelKeys: baseLabelKeys,
			},
			valueFunc: func() metricValues {
				return metricValues{{value: oomCount, metricType: types.MetricType_COUNTER}}
			},
		},
	}

	return computeSandboxMetrics(sb, oomMetrics, "oom")
}
