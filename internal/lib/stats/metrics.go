package statsserver

import (
	"time"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/oci"
)

var baseLabelKeys = []string{"id", "name", "image"}

const (
	CPUMetrics     = "cpu"
	HugetlbMetrics = "hugetlb"
	MemoryMetrics  = "memory"
	NetworkMetrics = "network"
	OOMMetrics     = "oom"
	ProcessMetrics = "process"
	SpecMetrics    = "spec"
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
		"": {
			containerLastSeen,
		},
		CPUMetrics: {
			containerCpuUserSecondsTotal,
			containerCpuSystemSecondsTotal,
			containerCpuUsageSecondsTotal,
			containerCpuCfsPeriodsTotal,
			containerCpuCfsThrottledPeriodsTotal,
			containerCpuCfsThrottledSecondsTotal,
		},
		HugetlbMetrics: {
			containerHugetlbUsageBytes,
			containerHugetlbMaxUsageBytes,
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
		ProcessMetrics: {
			containerProcesses,
		},
		SpecMetrics: {
			containerSpecCpuPeriod,
			containerSpecCpuShares,
			containerSpecCpuQuota,
			containerSpecMemoryLimitBytes,
			containerSpecMemoryReservationLimitBytes,
			containerSpecMemorySwapLimitBytes,
		},
	}

	return descriptorsMap
}

// ComputeSandboxMetrics computes the metrics for both pod and container sandbox.
func computeSandboxMetrics(sb *sandbox.Sandbox, metrics []*containerMetric, metricName string) []*types.Metric {
	return computeMetrics(sandboxBaseLabelValues(sb), metrics, metricName)
}

func sandboxBaseLabelValues(sb *sandbox.Sandbox) []string {
	// TODO FIXME: image?
	return []string{sb.ID(), "POD", ""}
}

// computeContainerMetrics computes the metrics for container.
func computeContainerMetrics(ctr *oci.Container, metrics []*containerMetric, metricName string) []*types.Metric {
	return computeMetrics(containerBaseLabelValues(ctr), metrics, metricName)
}

func containerBaseLabelValues(ctr *oci.Container) []string {
	image := ""
	if someNameOfTheImage := ctr.SomeNameOfTheImage(); someNameOfTheImage != nil {
		image = someNameOfTheImage.StringForOutOfProcessConsumptionOnly()
	}

	return []string{ctr.ID(), ctr.Name(), image}
}

func computeMetrics(baseLabels []string, metrics []*containerMetric, metricName string) []*types.Metric {
	if metricName != "" {
		baseLabels = append(baseLabels, metricName)
	}

	calculatedMetrics := make([]*types.Metric, 0, len(metrics))

	for _, m := range metrics {
		for _, v := range m.valueFunc() {
			labels := baseLabels

			if len(v.labels) > 0 {
				labels = make([]string, 0, len(baseLabels)+len(v.labels))
				labels = append(labels, baseLabels...)
				labels = append(labels, v.labels...)
			}

			newMetric := &types.Metric{
				Name:        m.desc.GetName(),
				Timestamp:   time.Now().UnixNano(),
				MetricType:  v.metricType,
				Value:       &types.UInt64Value{Value: v.value},
				LabelValues: labels,
			}
			calculatedMetrics = append(calculatedMetrics, newMetric)
		}
	}

	return calculatedMetrics
}
