package statsserver

import (
	"fmt"
	"time"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/opencontainers/runc/libcontainer/cgroups"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

var (
	cpuMetrics []ContainerStats = []ContainerStats{
		{
			desc: &types.MetricDescriptor{
				Name:      "container_cpu_user_seconds_total", // stats.CpuStats.CpuUsage.UsageInUsermode (converted from nano)
				Help:      "Cumulative user cpu time consumed in seconds.",
				LabelKeys: baseLabelKeys,
			},
			valueFunc: func(stats interface{}) metricValues {
				cpu := stats.(*cgroups.CpuStats)
				return metricValues{{
					value:      cpu.CpuUsage.UsageInUsermode / uint64(time.Second),
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: &types.MetricDescriptor{
				Name:      "container_cpu_system_seconds_total", // stats.CpuStats.CpuUsage.UsageInKernelmode (converted from nano)
				Help:      "Cumulative system cpu time consumed in seconds.",
				LabelKeys: baseLabelKeys,
			},
			valueFunc: func(stats interface{}) metricValues {
				cpu := stats.(*cgroups.CpuStats)
				return metricValues{{
					value:      cpu.CpuUsage.UsageInKernelmode / uint64(time.Second),
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: &types.MetricDescriptor{
				Name:      "container_cpu_usage_seconds_total",
				Help:      "Cumulative cpu time consumed in seconds.",
				LabelKeys: append(baseLabelKeys, "cpu"),
			},
			valueFunc: func(stats interface{}) metricValues {
				cpu := stats.(*cgroups.CpuStats)
				if len(cpu.CpuUsage.PercpuUsage) == 0 {
					if cpu.CpuUsage.TotalUsage > 0 {
						return metricValues{{
							value:      cpu.CpuUsage.TotalUsage / uint64(time.Second),
							labels:     []string{"total"},
							metricType: types.MetricType_COUNTER,
						}}
					}
				}
				metricValues := make(metricValues, 0, len(cpu.CpuUsage.PercpuUsage))
				for i, value := range cpu.CpuUsage.PercpuUsage {
					if value > 0 {
						metricValues = append(metricValues, metricValue{
							value:      value / uint64(time.Second),
							labels:     []string{fmt.Sprintf("cpu%02d", i)},
							metricType: types.MetricType_COUNTER,
						})
					}
				}
				return metricValues
			},
		}, {
			desc: &types.MetricDescriptor{
				Name:      "container_cpu_cfs_periods_total",
				Help:      "Number of elapsed enforcement period intervals.",
				LabelKeys: baseLabelKeys,
			},
			valueFunc: func(stats interface{}) metricValues {
				cpu := stats.(*cgroups.CpuStats)
				return metricValues{{
					value:      cpu.ThrottlingData.Periods,
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: &types.MetricDescriptor{
				Name:      "container_cpu_cfs_throttled_periods_total", // stats.CpuStats.ThrottlingData.ThrottledPeriods
				Help:      "Number of throttled period intervals.",
				LabelKeys: baseLabelKeys,
			},
			valueFunc: func(stats interface{}) metricValues {
				cpu := stats.(*cgroups.CpuStats)
				return metricValues{{
					value:      cpu.ThrottlingData.ThrottledPeriods,
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: &types.MetricDescriptor{
				Name:      "container_cpu_cfs_throttled_seconds_total", // stats.CpuStats.ThrottlingData.ThrottledTime (converted from nano)
				Help:      "Total time duration the container has been throttled.",
				LabelKeys: baseLabelKeys,
			},
			valueFunc: func(stats interface{}) metricValues {
				cpu := stats.(*cgroups.CpuStats)
				return metricValues{{
					value:      cpu.ThrottlingData.ThrottledTime / uint64(time.Second),
					metricType: types.MetricType_COUNTER,
				}}
			},
		},
	}
)

func GenerateSandboxCPUMetrics(sb *sandbox.Sandbox, c *cgroups.CpuStats, sm *SandboxMetrics) []*types.Metric {
	return ComputeSandboxMetrics(sb, c, cpuMetrics, "cpu", sm)
}
