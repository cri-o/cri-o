package statsserver

import (
	"github.com/vishvananda/netlink"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
)

func (ss *StatsServer) GenerateNetworkMetrics(sb *sandbox.Sandbox) []*types.Metric {
	var metrics []*types.Metric

	links, err := netlink.LinkList()
	if err != nil {
		log.Errorf(ss.ctx, "Unable to retrieve network namespace links %s: %v", sb.ID(), err)

		return nil
	}

	if len(links) == 0 {
		log.Warnf(ss.ctx, "Network links are not available.")

		return nil
	}

	for i := range links {
		if attrs := links[i].Attrs(); attrs != nil {
			networkMetrics := generateSandboxNetworkMetrics(sb, attrs)
			metrics = append(metrics, networkMetrics...)
		}
	}

	return metrics
}

func generateSandboxNetworkMetrics(sb *sandbox.Sandbox, attr *netlink.LinkAttrs) []*types.Metric {
	if attr == nil || attr.Statistics == nil {
		return []*types.Metric{}
	}

	networkMetrics := []*containerMetric{
		{
			desc: containerNetworkReceiveBytesTotal,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      attr.Statistics.RxBytes,
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: containerNetworkReceivePacketsTotal,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      attr.Statistics.RxPackets,
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: containerNetworkReceivePacketsDroppedTotal,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      attr.Statistics.RxDropped,
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: containerNetworkReceiveErrorsTotal,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      attr.Statistics.RxErrors,
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: containerNetworkTransmitBytesTotal,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      attr.Statistics.TxBytes,
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: containerNetworkTransmitPacketsTotal,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      attr.Statistics.TxPackets,
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: containerNetworkTransmitPacketsDroppedTotal,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      attr.Statistics.TxDropped,
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: containerNetworkTransmitErrorsTotal,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      attr.Statistics.TxErrors,
					metricType: types.MetricType_COUNTER,
				}}
			},
		},
	}

	return computeSandboxMetrics(sb, networkMetrics, "network")
}
