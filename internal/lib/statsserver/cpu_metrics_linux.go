package statsserver

import (
	"fmt"
	"time"

	"github.com/opencontainers/cgroups"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/oci"
)

func generateContainerCPUMetrics(ctr *oci.Container, cpu *cgroups.CpuStats) []*types.Metric {
	if cpu == nil {
		return []*types.Metric{}
	}

	cpuMetrics := []*containerMetric{
		{
			desc: containerCpuUserSecondsTotal,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      cpu.CpuUsage.UsageInUsermode / uint64(time.Second),
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: containerCpuSystemSecondsTotal,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      cpu.CpuUsage.UsageInKernelmode / uint64(time.Second),
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: containerCpuUsageSecondsTotal,
			valueFunc: func() metricValues {
				if len(cpu.CpuUsage.PercpuUsage) == 0 && cpu.CpuUsage.TotalUsage > 0 {
					return metricValues{{
						value:      cpu.CpuUsage.TotalUsage / uint64(time.Second),
						labels:     []string{"total"},
						metricType: types.MetricType_COUNTER,
					}}
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
			desc: containerCpuCfsPeriodsTotal,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      cpu.ThrottlingData.Periods,
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: containerCpuCfsThrottledPeriodsTotal,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      cpu.ThrottlingData.ThrottledPeriods,
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: containerCpuCfsThrottledSecondsTotal,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      cpu.ThrottlingData.ThrottledTime / uint64(time.Second),
					metricType: types.MetricType_COUNTER,
				}}
			},
		},
	}

	return computeContainerMetrics(ctr, cpuMetrics, "cpu")
}
