package statsserver

import (
	"time"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
)

var baseLabelKeys = []string{"id", "name", "image"}

const (
	CPUMetrics     = "cpu"
	MemoryMetrics  = "memory"
	NetworkMetrics = "network"
	OOMMetrics     = "oom"
)

type metricValue struct {
	value      uint64
	labels     []string
	metricType types.MetricType
}

type metricValues []metricValue

type containerMetric struct {
	desc      *types.MetricDescriptor
	valueFunc func() metricValues
}

type SandboxMetrics struct {
	metric *types.PodSandboxMetrics
}

func (s *SandboxMetrics) GetMetric() *types.PodSandboxMetrics {
	return s.metric
}

func NewSandboxMetrics(sb *sandbox.Sandbox) *SandboxMetrics {
	return &SandboxMetrics{
		metric: &types.PodSandboxMetrics{
			PodSandboxId:     sb.ID(),
			Metrics:          []*types.Metric{},
			ContainerMetrics: []*types.ContainerMetrics{},
		},
	}
}

// PopulateMetricDescriptors stores metricdescriptors statically at startup and populates the list.
func (ss *StatsServer) PopulateMetricDescriptors(includedKeys []string) map[string][]*types.MetricDescriptor {
	descriptorsMap := map[string][]*types.MetricDescriptor{
		CPUMetrics: {
			containerCpuUserSecondsTotal,
			containerCpuSystemSecondsTotal,
			containerCpuUsageSecondsTotal,
			containerCpuCfsPeriodsTotal,
			containerCpuCfsThrottledPeriodsTotal,
			containerCpuCfsThrottledSecondsTotal,
		},
		MemoryMetrics: {
			containerMemoryCache,
			containerMemoryRss,
			containerMemoryKernelUsage,
			containerMemoryMappedFile,
			containerMemorySwap,
			containerMemoryFailcnt,
			containerMemoryUsageBytes,
			containerMemoryMaxUsageBytes,
			containerMemoryWorkingSetBytes,
			containerMemoryFailuresTotal,
		},
		NetworkMetrics: {
			containerNetworkReceiveBytesTotal,
			containerNetworkReceivePacketsTotal,
			containerNetworkReceivePacketsDroppedTotal,
			containerNetworkReceiveErrorsTotal,
			containerNetworkTransmitBytesTotal,
			containerNetworkTransmitPacketsTotal,
			containerNetworkTransmitPacketsDroppedTotal,
			containerNetworkTransmitErrorsTotal,
		},
		OOMMetrics: {
			containerOomEventsTotal,
		},
	}

	return descriptorsMap
}

func sandboxBaseLabelValues(sb *sandbox.Sandbox) []string {
	// TODO FIXME: image?
	return []string{sb.ID(), "POD", ""}
}

// ComputeSandboxMetrics computes the metrics for both pod and container sandbox.
func computeSandboxMetrics(sb *sandbox.Sandbox, metrics []*containerMetric, metricName string) []*types.Metric {
	values := append(sandboxBaseLabelValues(sb), metricName)
	calculatedMetrics := make([]*types.Metric, 0, len(metrics))

	for _, m := range metrics {
		for _, v := range m.valueFunc() {
			newMetric := &types.Metric{
				Name:        m.desc.Name,
				Timestamp:   time.Now().UnixNano(),
				MetricType:  v.metricType,
				Value:       &types.UInt64Value{Value: v.value},
				LabelValues: append(values, v.labels...),
			}
			calculatedMetrics = append(calculatedMetrics, newMetric)
		}
	}

	return calculatedMetrics
}
