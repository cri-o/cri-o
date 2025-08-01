package statsserver

import (
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/config/cgmgr"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
)

func generateSandboxProcessMetrics(sb *sandbox.Sandbox, pids *cgmgr.PidsStats) []*types.Metric {
	processMetrics := []*containerMetric{
		{
			desc: containerProcesses,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      pids.Current,
					metricType: types.MetricType_GAUGE,
				}}
			},
		},
	}

	return computeSandboxMetrics(sb, processMetrics, "process")
}
