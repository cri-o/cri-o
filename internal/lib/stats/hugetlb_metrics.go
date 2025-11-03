package statsserver

import (
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/config/cgmgr"
	"github.com/cri-o/cri-o/internal/oci"
)

func generateContainerHugetlbMetrics(ctr *oci.Container, hugetlb map[string]cgmgr.HugetlbStats) []*types.Metric {
	hugetlbMetrics := []*containerMetric{
		{
			desc: containerHugetlbUsageBytes,
			valueFunc: func() metricValues {
				metricValues := make(metricValues, 0, len(hugetlb))
				for pagesize, stat := range hugetlb {
					metricValues = append(metricValues, metricValue{
						value:      stat.Usage,
						labels:     []string{pagesize},
						metricType: types.MetricType_GAUGE,
					})
				}

				return metricValues
			},
		}, {
			desc: containerHugetlbMaxUsageBytes,
			valueFunc: func() metricValues {
				metricValues := make(metricValues, 0, len(hugetlb))
				for pagesize, stat := range hugetlb {
					metricValues = append(metricValues, metricValue{
						value:      stat.Max,
						labels:     []string{pagesize},
						metricType: types.MetricType_GAUGE,
					})
				}

				return metricValues
			},
		},
	}

	return computeContainerMetrics(ctr, hugetlbMetrics, "hugetlb")
}
