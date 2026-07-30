package statsserver

import (
	"fmt"

	"github.com/prometheus/procfs"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
)

func (ss *StatsServer) GenerateNetworkMetrics(sb *sandbox.Sandbox) []*types.Metric {
	if sb.HostNetwork() {
		return ss.generateHostNetworkMetrics(sb)
	}

	return ss.generatePodNetworkMetrics(sb)
}

// generateHostNetworkMetrics returns metrics from the pre-computed host
// network dev snapshot, which already has pod-owned interfaces filtered out.
func (ss *StatsServer) generateHostNetworkMetrics(sb *sandbox.Sandbox) []*types.Metric {
	var metrics []*types.Metric

	for name := range ss.hostNetDev {
		iface := ss.hostNetDev[name]
		networkMetrics := generateSandboxNetworkMetrics(sb, &iface)
		metrics = append(metrics, networkMetrics...)
	}

	return metrics
}

func (ss *StatsServer) generatePodNetworkMetrics(sb *sandbox.Sandbox) []*types.Metric {
	netDev, err := readPodNetDev(sb)
	if err != nil {
		log.WithFields(ss.ctx, map[string]any{
			"sandboxID": sb.ID(),
			"error":     err,
		}).Error("Unable to retrieve network stats")

		return nil
	}

	if len(netDev) == 0 {
		log.Warnf(ss.ctx, "Network links are not available.")

		return nil
	}

	var metrics []*types.Metric

	for name := range netDev {
		iface := netDev[name]
		networkMetrics := generateSandboxNetworkMetrics(sb, &iface)
		metrics = append(metrics, networkMetrics...)
	}

	return metrics
}

func readPodNetDev(sb *sandbox.Sandbox) (procfs.NetDev, error) {
	var lastErr error

	for _, ctr := range sb.Containers().List() {
		pid, err := ctr.Pid()
		if err != nil {
			continue
		}

		proc, err := procfs.NewProc(pid)
		if err != nil {
			continue
		}

		netDev, err := proc.NetDev()
		if err != nil {
			lastErr = err

			continue
		}

		return netDev, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("read network stats for sandbox %s: %w", sb.ID(), lastErr)
	}

	return nil, fmt.Errorf("no running container in sandbox %s", sb.ID())
}

func generateSandboxNetworkMetrics(sb *sandbox.Sandbox, iface *procfs.NetDevLine) []*types.Metric {
	networkMetrics := []*containerMetric{
		{
			desc: containerNetworkReceiveBytesTotal,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      iface.RxBytes,
					labels:     []string{iface.Name},
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: containerNetworkReceivePacketsTotal,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      iface.RxPackets,
					labels:     []string{iface.Name},
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: containerNetworkReceivePacketsDroppedTotal,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      iface.RxDropped,
					labels:     []string{iface.Name},
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: containerNetworkReceiveErrorsTotal,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      iface.RxErrors,
					labels:     []string{iface.Name},
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: containerNetworkTransmitBytesTotal,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      iface.TxBytes,
					labels:     []string{iface.Name},
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: containerNetworkTransmitPacketsTotal,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      iface.TxPackets,
					labels:     []string{iface.Name},
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: containerNetworkTransmitPacketsDroppedTotal,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      iface.TxDropped,
					labels:     []string{iface.Name},
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: containerNetworkTransmitErrorsTotal,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      iface.TxErrors,
					labels:     []string{iface.Name},
					metricType: types.MetricType_COUNTER,
				}}
			},
		},
	}

	return computeSandboxMetrics(sb, networkMetrics)
}
