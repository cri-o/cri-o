package statsserver

import (
	"os"
	"fmt"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
)

func generateSandboxProcessMetrics(sb *sandbox.Sandbox, pid int) []*types.Metric {
	processMetrics := []*containerMetric{
		{
			desc: containerFileDescriptors,
			valueFunc: func() metricValues {
				fdDir := fmt.Sprintf("/proc/%d/fd", pid)
				entries, err := os.ReadDir(fdDir)
				if err != nil {
					return metricValues{}
				}

				return metricValues{{
					value: uint64(len(entries)),
					metricType: types.MetricType_GAUGE,
				}}
			},
		},
	}

	return computeSandboxMetrics(sb, processMetrics, "process")
}
