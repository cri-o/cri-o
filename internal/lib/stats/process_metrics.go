package statsserver

import (
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/config/cgmgr"
	"github.com/cri-o/cri-o/internal/oci"
)

func generateContainerProcessMetrics(ctr *oci.Container, pids *cgmgr.PidsStats) []*types.Metric {
	processMetrics := []*containerMetric{
		{
			desc: containerFileDescriptors,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      pids.FileDescriptors,
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
					value:      pids.Sockets,
					metricType: types.MetricType_GAUGE,
				}}
			},
		},
		{
			desc: containerThreads,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      pids.Threads,
					metricType: types.MetricType_GAUGE,
				}}
			},
		},
		{
			desc: containerThreadsMax,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      pids.ThreadsMax,
					metricType: types.MetricType_GAUGE,
				}}
			},
		},
		{
			desc: containerUlimitsSoft,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      pids.UlimitsSoft,
					metricType: types.MetricType_GAUGE,
				}}
			},
		},
	}

	return computeContainerMetrics(ctr, processMetrics, "process")
}
