package statsserver

import (
	"context"
	"strconv"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
)

func generateContainerSpecMetrics(ctr *oci.Container) []*types.Metric {
	resources := ctr.GetResources()
	if resources == nil || resources.GetLinux() == nil {
		return []*types.Metric{}
	}

	specMetrics := []*containerMetric{
		{
			desc: containerSpecMemoryLimitBytes,
			valueFunc: func() metricValues {
				return metricValues{{value: specMemoryValue(resources.GetLinux().GetMemoryLimitInBytes()), metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: containerSpecMemorySwapLimitBytes,
			valueFunc: func() metricValues {
				return metricValues{{value: specMemoryValue(resources.GetLinux().GetMemorySwapLimitInBytes()), metricType: types.MetricType_GAUGE}}
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
		u, err := strconv.ParseInt(m, 10, 64)
		if err != nil {
			log.Errorf(context.Background(), "Failed to parse memory.min value %s: %v", m, err)
		} else {
			specMetrics = append(specMetrics, &containerMetric{
				desc: containerSpecMemoryReservationLimitBytes,
				valueFunc: func() metricValues {
					return metricValues{{value: specMemoryValue(u), metricType: types.MetricType_GAUGE}}
				},
			})
		}
	}

	return computeContainerMetrics(ctr, specMetrics, "spec")
}

func specMemoryValue(limit int64) uint64 {
	// Size after which we consider memory to be "unlimited". This is not
	// MaxInt64 due to rounding by the kernel.
	const maxMemorySize = uint64(1 << 62)

	// For consistency with cAdvisor and Kubernetes, consider memory to be "unlimited"
	// when above the maxMemorySize and report it as 0 in the metrics.
	// This approach is more useful for monitoring tools than reporting the physical limit.
	// Also add negative handling here, as a negative limit means unlimited as well.
	if limit < 0 || uint64(limit) > maxMemorySize {
		return 0
	}

	return uint64(limit)
}
