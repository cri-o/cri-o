package statsserver

import (
	"fmt"

	"github.com/prometheus/procfs"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
)

func (ss *StatsServer) GenerateNetworkMetrics(sb *sandbox.Sandbox) []*types.Metric {
	netDev, err := ss.readNetDev(sb)
	if err != nil {
		log.Errorf(ss.ctx, "Unable to retrieve network stats %s: %v", sb.ID(), err)

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

func (ss *StatsServer) readNetDev(sb *sandbox.Sandbox) (procfs.NetDev, error) {
	if sb.HostNetwork() {
		proc, err := procfs.Self()
		if err != nil {
			return nil, err
		}

		return proc.NetDev()
	}

	for _, ctr := range sb.Containers().List() {
		pid, err := ctr.Pid()
		if err != nil {
			continue
		}

		proc, err := procfs.NewProc(pid)
		if err != nil {
			continue
		}

		return proc.NetDev()
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
