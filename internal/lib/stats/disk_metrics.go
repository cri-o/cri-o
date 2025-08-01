package statsserver

import (
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/config/cgmgr"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/oci"
)

// generateSandboxDiskMetrics computes filesystem disk metrics from DiskMetrics for a container sandbox.
func generateSandboxDiskMetrics(sb *sandbox.Sandbox, ctr *oci.Container, diskStats *cgmgr.DiskMetrics) []*types.Metric {
	diskMetrics := []*containerMetric{
		{
			desc: containerFsInodesFree,
			valueFunc: func() metricValues {
				return metricValues{{value: diskStats.InodesFree, metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: containerFsInodesTotal,
			valueFunc: func() metricValues {
				return metricValues{{value: diskStats.InodesTotal, metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: containerFsLimitBytes,
			valueFunc: func() metricValues {
				return metricValues{{value: diskStats.LimitBytes, metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: containerFsUsageBytes,
			valueFunc: func() metricValues {
				return metricValues{{value: diskStats.UsageBytes, metricType: types.MetricType_GAUGE}}
			},
		},
	}

	return computeSandboxMetrics(sb, diskMetrics, "disk")
}
