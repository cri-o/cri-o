package statsserver

import (
	"syscall"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/oci"
)

// generateSandboxDiskMetrics computes filesystem disk metrics for a container sandbox.
func generateSandboxDiskMetrics(sb *sandbox.Sandbox, ctr *oci.Container, fsStats *syscall.Statfs_t, usageBytes uint64) []*types.Metric {

	diskMetrics := []*containerMetric{
		{
			desc: containerFsInodesFree,
			valueFunc: func() metricValues {
				return metricValues{{value: uint64(fsStats.Ffree), metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: containerFsInodesTotal,
			valueFunc: func() metricValues {
				return metricValues{{value: uint64(fsStats.Files), metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: containerFsLimitBytes,
			valueFunc: func() metricValues {
				return metricValues{{value: uint64(fsStats.Blocks) * uint64(fsStats.Bsize), metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: containerFsUsageBytes,
			valueFunc: func() metricValues {
				return metricValues{{value: usageBytes, metricType: types.MetricType_GAUGE}}
			},
		},
	}

	return computeSandboxMetrics(sb, diskMetrics, "disk")
}
