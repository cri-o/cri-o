package statsserver

import (
	"github.com/opencontainers/cgroups"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/lib/stats"
	"github.com/cri-o/cri-o/internal/oci"
)

func generateContainerProcessMetrics(ctr *oci.Container, pids *cgroups.PidsStats, process *stats.ProcessStats) []*types.Metric {
	if pids == nil || process == nil {
		return []*types.Metric{}
	}

	processMetrics := []*containerMetric{
		{
			desc: containerFileDescriptors,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      process.FileDescriptors,
					metricType: types.MetricType_GAUGE,
				}}
			},
		},
		{
			desc: containerProcesses,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      pids.Current,
					metricType: types.MetricType_GAUGE,
				}}
			},
		},
		{
			desc: containerSockets,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      process.Sockets,
					metricType: types.MetricType_GAUGE,
				}}
			},
		},
		{
			desc: containerThreads,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      process.Threads,
					metricType: types.MetricType_GAUGE,
				}}
			},
		},
		{
			desc: containerThreadsMax,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      process.ThreadsMax,
					metricType: types.MetricType_GAUGE,
				}}
			},
		},
		{
			desc: containerUlimitsSoft,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      process.UlimitsSoft,
					metricType: types.MetricType_GAUGE,
				}}
			},
		},
	}

	return computeContainerMetrics(ctr, processMetrics, "process")
}
