package statsserver

import (
	
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	
)

func generateSandboxOtherMetrics(sb *sandbox.Sandbox, others *oci.ContainerState) []*types.Metric {
	otherMetrics := []*containerMetric{
		{
			desc: containerStartTimeSeconds,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      uint64(others.Started.Unix()) ,
					metricType: types.MetricType_GAUGE,
				}}
			},
		},
	}

	return computeSandboxMetrics(sb, otherMetrics, "other")
}