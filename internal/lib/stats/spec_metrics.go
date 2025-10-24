package statsserver

import (
	"strconv"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/oci"
)

func generateSandboxSpecMetrics(sb *sandbox.Sandbox, c *oci.Container) []*types.Metric {
	// TODO FIXME read lock
	resources := c.GetResources()
	if resources.GetLinux() == nil {
		return []*types.Metric{}
	}

	specMetrics := []*containerMetric{
		{
			desc: containerSpecMemoryLimitBytes,
			valueFunc: func() metricValues {
				// For consistency with cAdvisor and Kubernetes, consider memory to be "unlimited"
				// when above a certain threshold (2^62) and report it as 0 in the metrics.
				// This approach is more useful for monitoring tools than reporting the physical limit.
				limit := resources.GetLinux().GetMemoryLimitInBytes()
				if limit > int64(maxMemorySize) {
					return metricValues{{value: 0, metricType: types.MetricType_GAUGE}}
				}

				return metricValues{{value: uint64(limit), metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: containerSpecMemorySwapLimitBytes,
			valueFunc: func() metricValues {
				return metricValues{{value: uint64(resources.GetLinux().GetMemorySwapLimitInBytes()), metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: containerSpecCpuShares,
			valueFunc: func() metricValues {
				return metricValues{{value: uint64(resources.GetLinux().GetCpuShares()), metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: containerSpecCpuPeriod,
			valueFunc: func() metricValues {
				return metricValues{{value: uint64(resources.GetLinux().GetCpuPeriod()), metricType: types.MetricType_GAUGE}}
			},
		},
	}
	if resources.GetLinux().GetCpuQuota() > 0 {
		specMetrics = append(specMetrics, &containerMetric{
			desc: containerSpecCpuQuota,
			valueFunc: func() metricValues {
				return metricValues{{value: uint64(resources.GetLinux().GetCpuQuota()), metricType: types.MetricType_GAUGE}}
			},
		})
	}

	if m, ok := resources.GetLinux().GetUnified()["memory.min"]; ok {
		u, err := strconv.ParseUint(m, 10, 64)
		if err == nil {
			specMetrics = append(specMetrics, &containerMetric{
				desc: containerSpecMemoryReservationLimitBytes,
				valueFunc: func() metricValues {
					return metricValues{{value: u, metricType: types.MetricType_GAUGE}}
				},
			})
		}
	}

	return computeSandboxMetrics(sb, specMetrics, "spec")
}
