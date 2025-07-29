package statsserver

import (
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/config/cgmgr"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
)

func generateSandboxProcessMetrics(sb *sandbox.Sandbox, process *cgmgr.ProcessStats) []*types.Metric {
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
	}

	return computeSandboxMetrics(sb, processMetrics, "process")
}
