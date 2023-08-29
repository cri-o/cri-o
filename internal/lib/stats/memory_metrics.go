package statsserver

import (
	"github.com/cri-o/cri-o/internal/config/node"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/opencontainers/runc/libcontainer/cgroups"
	"github.com/sirupsen/logrus"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func GenerateSandboxMemoryMetrics(sb *sandbox.Sandbox, stats interface{}, sm *SandboxMetrics) []*types.Metric {
	mem, ok := stats.(*cgroups.MemoryStats)
	if !ok {
		logrus.Errorf("Failed to assert stats as *cgroups.MemoryStats")
		return nil
	}
	memoryMetrics := []*ContainerStats{
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
				var value uint64
				switch {
				case cgroups.IsCgroup2UnifiedMode():
					value = mem.Stats["anon"]
				case mem.UseHierarchy:
					value = mem.Stats["total_rss"]
				default:
					value = mem.Stats["rss"]
				}
				return metricValues{{value: value, metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: &types.MetricDescriptor{
				Name:      "container_memory_kernel_usage",
				Help:      "Size of kernel memory allocated in bytes.",
				LabelKeys: baseLabelKeys,
			},
			valueFunc: func() metricValues {
				return metricValues{{value: mem.KernelUsage.Usage, metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: &types.MetricDescriptor{
				Name:      "container_memory_mapped_file",
				Help:      "Size of memory mapped files in bytes.",
				LabelKeys: baseLabelKeys,
			},
			valueFunc: func() metricValues {
				var value uint64
				switch {
				case node.CgroupIsV2():
					value = mem.Stats["file_mapped"]
				case mem.UseHierarchy:
					value = mem.Stats["total_mapped_file"]
				default:
					value = mem.Stats["mapped_file"]
				}
				return metricValues{{value: value, metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: &types.MetricDescriptor{
				Name:      "container_memory_swap",
				Help:      "Container swap usage in bytes.",
				LabelKeys: baseLabelKeys,
			},
			valueFunc: func() metricValues {
				return metricValues{{value: mem.SwapUsage.Usage, metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: &types.MetricDescriptor{
				Name:      "container_memory_failcnt",
				Help:      "Number of memory usage hits limits",
				LabelKeys: baseLabelKeys,
			},
			valueFunc: func() metricValues {
				return metricValues{{value: mem.SwapUsage.Failcnt, metricType: types.MetricType_COUNTER}}
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
					value:      mem.Usage.Usage,
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
					value:      mem.Usage.MaxUsage,
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
				var workingSet uint64
				inactiveFileKeyName := "total_inactive_file"
				if node.CgroupIsV2() {
					inactiveFileKeyName = "inactive_file"
				}
				workingSet = mem.Usage.Usage
				if v, ok := mem.Stats[inactiveFileKeyName]; ok {
					if workingSet < v {
						workingSet = 0
					} else {
						workingSet -= v
					}
				}
				return metricValues{{value: workingSet, metricType: types.MetricType_GAUGE}}
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
				if v, ok := mem.Stats["pgfault"]; ok {
					pgfaultMetrics := metricValues{
						{
							value:      v,
							labels:     []string{"pgfault", "container"},
							metricType: types.MetricType_COUNTER,
						},
						{
							value:      v,
							labels:     []string{"pgfault", "hierarchy"},
							metricType: types.MetricType_COUNTER,
						},
					}

					metrics = append(metrics, pgfaultMetrics...)
				}

				if v, ok := mem.Stats["pgmajfault"]; ok {
					pgmajfaultMetrics := metricValues{
						{
							value:      v,
							labels:     []string{"pgmajfault", "container"},
							metricType: types.MetricType_COUNTER,
						},
						{
							value:      v,
							labels:     []string{"pgmajfault", "hierarchy"},
							metricType: types.MetricType_COUNTER,
						},
					}
					metrics = append(metrics, pgmajfaultMetrics...)
				}
				return metrics
			},
		},
	}
	return ComputeSandboxMetrics(sb, memoryMetrics, "memory", sm)
}

func GenerateSandboxOOMMetrics(sb *sandbox.Sandbox, stats interface{}, sm *SandboxMetrics) []*types.Metric {
	oom, ok := stats.(cgroups.Manager)
	if !ok {
		logrus.Errorf("Failed to assert stats as cgroups.Manager")
		return nil
	}
	oomMetrics := []*ContainerStats{
		{
			desc: &types.MetricDescriptor{
				Name:      "container_oom_events_total",
				Help:      "Count of out of memory events observed for the container",
				LabelKeys: baseLabelKeys,
			},
			valueFunc: func() metricValues {
				count, err := oom.OOMKillCount()
				if err != nil {
					count = 0
				}
				return metricValues{{value: count, metricType: types.MetricType_COUNTER}}
			},
		},
	}
	return ComputeSandboxMetrics(sb, oomMetrics, "oom", sm)
}
