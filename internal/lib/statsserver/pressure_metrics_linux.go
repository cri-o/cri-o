package statsserver

import (
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/config/cgmgr"
	"github.com/cri-o/cri-o/internal/oci"
)

func microSecondsToSeconds(microSeconds uint64) uint64 {
	return microSeconds / 1e6
}

// generateContainerPressureMetrics generates metrics for pressure stalling events
// It returns PSI Total time in seconds for each pressure type.
// Because cAdvisor returns in seconds, we convert to seconds here, though we can't
// return float64 because of the CRI API spec.
// https://github.com/google/cadvisor/pull/3649/files#diff-583dd1a38478c42e7ee4f90a9c3dfb5fd8a07b82f57d4ed24fa6a98a5951a4e7R1754
func generateContainerPressureMetrics(ctr *oci.Container, cpu *cgmgr.CPUStats, memory *cgmgr.MemoryStats, blkio *cgmgr.DiskIOStats) []*types.Metric {
	var metrics []*containerMetric

	if cpu != nil && cpu.PSI != nil {
		metrics = append(metrics,
			&containerMetric{
				desc: containerPressureCPUStalledSecondsTotal,
				valueFunc: func() metricValues {
					return metricValues{{
						value:      microSecondsToSeconds(cpu.PSI.Full.Total),
						metricType: types.MetricType_COUNTER,
					}}
				},
			},
			&containerMetric{
				desc: containerPressureCPUWaitingSecondsTotal,
				valueFunc: func() metricValues {
					return metricValues{{
						value:      microSecondsToSeconds(cpu.PSI.Some.Total),
						metricType: types.MetricType_COUNTER,
					}}
				},
			},
		)
	}

	if memory != nil && memory.PSI != nil {
		metrics = append(metrics,
			&containerMetric{
				desc: containerPressureMemoryStalledSecondsTotal,
				valueFunc: func() metricValues {
					return metricValues{{
						value:      microSecondsToSeconds(memory.PSI.Full.Total),
						metricType: types.MetricType_COUNTER,
					}}
				},
			},
			&containerMetric{
				desc: containerPressureMemoryWaitingSecondsTotal,
				valueFunc: func() metricValues {
					return metricValues{{
						value:      microSecondsToSeconds(memory.PSI.Some.Total),
						metricType: types.MetricType_COUNTER,
					}}
				},
			},
		)
	}

	if blkio != nil && blkio.PSI != nil {
		metrics = append(metrics,
			&containerMetric{
				desc: containerPressureIOStalledSecondsTotal,
				valueFunc: func() metricValues {
					return metricValues{{
						value:      microSecondsToSeconds(blkio.PSI.Full.Total),
						metricType: types.MetricType_COUNTER,
					}}
				},
			},
			&containerMetric{
				desc: containerPressureIOWaitingSecondsTotal,
				valueFunc: func() metricValues {
					return metricValues{{
						value:      microSecondsToSeconds(blkio.PSI.Some.Total),
						metricType: types.MetricType_COUNTER,
					}}
				},
			},
		)
	}

	return computeContainerMetrics(ctr, metrics, "cpu")
}
