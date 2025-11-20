package statsserver

import (
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/config/cgmgr"
	"github.com/cri-o/cri-o/internal/oci"
)

func generateContainerPressureMetrics(ctr *oci.Container, cpu *cgmgr.CPUStats, memory *cgmgr.MemoryStats, blkio *cgmgr.DiskIOStats) []*types.Metric {
	metrics := []*containerMetric{
		{
			desc: containerPressureCPUStalledSecondsTotal,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      cpu.PSI.Full.Total,
					metricType: types.MetricType_COUNTER,
				}}
			},
		},
		{
			desc: containerPressureCPUWaitingSecondsTotal,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      cpu.PSI.Some.Total,
					metricType: types.MetricType_COUNTER,
				}}
			},
		},
		{
			desc: containerPressureMemoryStalledSecondsTotal,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      memory.PSI.Full.Total,
					metricType: types.MetricType_COUNTER,
				}}
			},
		},
		{
			desc: containerPressureMemoryWaitingSecondsTotal,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      memory.PSI.Some.Total,
					metricType: types.MetricType_COUNTER,
				}}
			},
		},
		{
			desc: containerPressureIOStalledSecondsTotal,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      blkio.PSI.Full.Total,
					metricType: types.MetricType_COUNTER,
				}}
			},
		},
		{
			desc: containerPressureIOWaitingSecondsTotal,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      blkio.PSI.Some.Total,
					metricType: types.MetricType_COUNTER,
				}}
			},
		},
	}

	return computeContainerMetrics(ctr, metrics, "cpu")
}
