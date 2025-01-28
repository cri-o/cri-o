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
	networkMetrics := []*containerMetric{
		{
			desc: &types.MetricDescriptor{
				Name:      "container_network_receive_bytes_total",
				Help:      "Cumulative count of bytes received",
				LabelKeys: append(baseLabelKeys, "interface"),
			},
			valueFunc: func() metricValues {
				return metricValues{{
					value:      attr.Statistics.RxBytes,
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: &types.MetricDescriptor{
				Name:      "container_network_receive_packets_total",
				Help:      "Cumulative count of packets received",
				LabelKeys: append(baseLabelKeys, "interface"),
			},
			valueFunc: func() metricValues {
				return metricValues{{
					value:      attr.Statistics.RxPackets,
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: &types.MetricDescriptor{
				Name:      "container_network_receive_packets_dropped_total",
				Help:      "Cumulative count of packets dropped while receiving",
				LabelKeys: append(baseLabelKeys, "interface"),
			},
			valueFunc: func() metricValues {
				return metricValues{{
					value:      attr.Statistics.RxDropped,
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: &types.MetricDescriptor{
				Name:      "container_network_receive_errors_total",
				Help:      "Cumulative count of errors encountered while receiving",
				LabelKeys: append(baseLabelKeys, "interface"),
			},
			valueFunc: func() metricValues {
				return metricValues{{
					value:      attr.Statistics.RxErrors,
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: &types.MetricDescriptor{
				Name:      "container_network_transmit_bytes_total",
				Help:      "Cumulative count of bytes transmitted",
				LabelKeys: append(baseLabelKeys, "interface"),
			},
			valueFunc: func() metricValues {
				return metricValues{{
					value:      attr.Statistics.TxBytes,
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: &types.MetricDescriptor{
				Name:      "container_network_transmit_packets_total",
				Help:      "Cumulative count of packets transmitted",
				LabelKeys: append(baseLabelKeys, "interface"),
			},
			valueFunc: func() metricValues {
				return metricValues{{
					value:      attr.Statistics.TxPackets,
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: &types.MetricDescriptor{
				Name:      "container_network_transmit_packets_dropped_total",
				Help:      "Cumulative count of packets dropped while transmitting",
				LabelKeys: append(baseLabelKeys, "interface"),
			},
			valueFunc: func() metricValues {
				return metricValues{{
					value:      attr.Statistics.TxDropped,
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: &types.MetricDescriptor{
				Name:      "container_network_transmit_errors_total",
				Help:      "Cumulative count of errors encountered while transmitting",
				LabelKeys: append(baseLabelKeys, "interface"),
			},
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
