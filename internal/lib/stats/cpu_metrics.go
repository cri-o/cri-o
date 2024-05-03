package statsserver

import (
	"fmt"
	"time"

	"github.com/cri-o/cri-o/internal/config/cgmgr"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func generateSandboxCPUMetrics(sb *sandbox.Sandbox, cpu *cgmgr.CPUStats) []*types.Metric {
	cpuMetrics := []*containerMetric{
		{
			desc: &types.MetricDescriptor{
				Name:      "container_cpu_user_seconds_total",
				Help:      "Cumulative user CPU time consumed in seconds.",
				LabelKeys: baseLabelKeys,
			},
			valueFunc: func() metricValues {
				return metricValues{{
					value:      cpu.UsageInUsermode / uint64(time.Second),
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: &types.MetricDescriptor{
				Name:      "container_cpu_system_seconds_total",
				Help:      "Cumulative system CPU time consumed in seconds.",
				LabelKeys: baseLabelKeys,
			},
			valueFunc: func() metricValues {
				return metricValues{{
					value:      cpu.UsageInKernelmode / uint64(time.Second),
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: &types.MetricDescriptor{
				Name:      "container_cpu_usage_seconds_total",
				Help:      "Cumulative CPU time consumed in seconds.",
				LabelKeys: append(baseLabelKeys, "cpu"),
			},
			valueFunc: func() metricValues {
				if len(cpu.PerCPUUsage) == 0 && cpu.TotalUsageNano > 0 {
					return metricValues{{
						value:      cpu.TotalUsageNano / uint64(time.Second),
						labels:     []string{"total"},
						metricType: types.MetricType_COUNTER,
					}}
				}
				metricValues := make(metricValues, 0, len(cpu.PerCPUUsage))
				for i, value := range cpu.PerCPUUsage {
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
			valueFunc: func() metricValues {
				return metricValues{{
					value:      cpu.ThrottlingActivePeriods,
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: &types.MetricDescriptor{
				Name:      "container_cpu_cfs_throttled_periods_total",
				Help:      "Number of throttled period intervals.",
				LabelKeys: baseLabelKeys,
			},
			valueFunc: func() metricValues {
				return metricValues{{
					value:      cpu.ThrottledPeriods,
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: &types.MetricDescriptor{
				Name:      "container_cpu_cfs_throttled_seconds_total",
				Help:      "Total time duration the container has been throttled.",
				LabelKeys: baseLabelKeys,
			},
			valueFunc: func() metricValues {
				return metricValues{{
					value:      cpu.ThrottledTime / uint64(time.Second),
					metricType: types.MetricType_COUNTER,
				}}
			},
		},
	}
	return computeSandboxMetrics(sb, cpuMetrics, "cpu")
}
