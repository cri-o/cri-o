package statsserver

import (
	"testing"
	"time"

	"github.com/opencontainers/cgroups"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/lib/stats"
	"github.com/cri-o/cri-o/internal/oci"
)

// TestMetricLabelCardinality verifies that every metric produced by each
// generate function has exactly as many label values as its descriptor
// declares label keys. A mismatch causes the kubelet's prometheus client
// to reject the metric with "inconsistent label cardinality".
func TestMetricLabelCardinality(t *testing.T) {
	t.Parallel()

	descLabelCount := descriptorLabelCounts()
	ctr := newTestContainer(t)

	cpuStats := testCPUStats()
	memStats := testMemoryStats()
	blkioStats := testBlkioStats()
	pidsStats := cgroups.PidsStats{Current: 10}
	processStats := stats.ProcessStats{
		FileDescriptors: 50,
		Sockets:         5,
		Threads:         10,
		ThreadsMax:      1000,
		UlimitsSoft:     1024,
	}
	diskStats := stats.FilesystemStats{
		UsageBytes:  1024 * 1024,
		LimitBytes:  10 * 1024 * 1024,
		InodesFree:  1000,
		InodesTotal: 2000,
	}
	hugetlbStats := map[string]cgroups.HugetlbStats{
		"2MB": {Usage: 1024, MaxUsage: 2048},
	}
	networkMetrics := testNetworkMetrics()

	tests := []struct {
		name    string
		metrics []*types.Metric
	}{
		{"cpu", generateContainerCPUMetrics(ctr, &cpuStats)},
		{"disk", generateContainerDiskMetrics(ctr, &diskStats)},
		{"diskIO", generateContainerDiskIOMetrics(ctr, &blkioStats)},
		{"hugetlb", generateContainerHugetlbMetrics(ctr, hugetlbStats)},
		{"memory", generateContainerMemoryMetrics(ctr, &memStats)},
		{"network", networkMetrics},
		{"oom", GenerateContainerOOMMetrics(ctr, 3)},
		{"process", generateContainerProcessMetrics(ctr, &pidsStats, &processStats)},
		{"spec", generateContainerSpecMetrics(ctr)},
		{"pressure", generateContainerPressureMetrics(ctr, &cpuStats, &memStats, &blkioStats)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if len(tt.metrics) == 0 {
				t.Fatal("no metrics generated")
			}

			for _, m := range tt.metrics {
				expected, ok := descLabelCount[m.GetName()]
				if !ok {
					t.Errorf("metric %q not found in availableMetricDescriptors", m.GetName())

					continue
				}

				if got := len(m.GetLabelValues()); got != expected {
					t.Errorf("metric %q: got %d label values, want %d (descriptor label keys: %d)",
						m.GetName(), got, expected, expected)
				}
			}
		})
	}
}

// descriptorLabelCounts builds a lookup from metric name to expected label count.
func descriptorLabelCounts() map[string]int {
	m := make(map[string]int)

	for _, descs := range availableMetricDescriptors {
		for _, d := range descs {
			m[d.GetName()] = len(d.GetLabelKeys())
		}
	}

	return m
}

func newTestContainer(t *testing.T) *oci.Container {
	t.Helper()

	ctr, err := oci.NewContainer(
		"test-id", "test-name", "", "", nil, nil, nil,
		"", nil, nil, "", &types.ContainerMetadata{}, "",
		false, false, false, "", "", time.Now(), "",
	)
	if err != nil {
		t.Fatalf("failed to create test container: %v", err)
	}

	cpuPeriod := uint64(100000)
	cpuQuota := int64(50000)
	cpuShares := uint64(1024)
	memLimit := int64(256 * 1024 * 1024)

	ctr.SetResources(&specs.Spec{
		Linux: &specs.Linux{
			Resources: &specs.LinuxResources{
				CPU: &specs.LinuxCPU{
					Period: &cpuPeriod,
					Quota:  &cpuQuota,
					Shares: &cpuShares,
				},
				Memory: &specs.LinuxMemory{
					Limit: &memLimit,
				},
			},
		},
	})

	return ctr
}

func testCPUStats() cgroups.CpuStats {
	return cgroups.CpuStats{
		CpuUsage: cgroups.CpuUsage{
			TotalUsage:        1000000000,
			UsageInUsermode:   500000000,
			UsageInKernelmode: 500000000,
			PercpuUsage:       []uint64{500000000, 500000000},
		},
		ThrottlingData: cgroups.ThrottlingData{
			Periods:          10,
			ThrottledPeriods: 2,
			ThrottledTime:    100000000,
		},
		PSI: &cgroups.PSIStats{
			Full: cgroups.PSIData{Total: 1000000},
			Some: cgroups.PSIData{Total: 2000000},
		},
	}
}

func testMemoryStats() cgroups.MemoryStats {
	return cgroups.MemoryStats{
		Usage:       cgroups.MemoryData{Usage: 1024 * 1024, MaxUsage: 2 * 1024 * 1024, Failcnt: 1},
		SwapUsage:   cgroups.MemoryData{Usage: 512 * 1024},
		KernelUsage: cgroups.MemoryData{Usage: 256 * 1024},
		Cache:       128 * 1024,
		Stats: map[string]uint64{
			"total_rss":           512 * 1024,
			"total_inactive_file": 64 * 1024,
			"total_mapped_file":   32 * 1024,
			"pgfault":             100,
			"pgmajfault":          5,
		},
		PSI: &cgroups.PSIStats{
			Full: cgroups.PSIData{Total: 500000},
			Some: cgroups.PSIData{Total: 1000000},
		},
	}
}

// testNetworkMetrics builds network metrics using computeMetrics directly,
// mirroring what generateSandboxNetworkMetrics does but without requiring a
// real sandbox.Sandbox or netlink.LinkAttrs.
func testNetworkMetrics() []*types.Metric {
	sandboxBaseLabels := []string{"test-sandbox-id", "POD", ""}

	networkDescs := []*types.MetricDescriptor{
		containerNetworkReceiveBytesTotal,
		containerNetworkReceivePacketsTotal,
		containerNetworkReceivePacketsDroppedTotal,
		containerNetworkReceiveErrorsTotal,
		containerNetworkTransmitBytesTotal,
		containerNetworkTransmitPacketsTotal,
		containerNetworkTransmitPacketsDroppedTotal,
		containerNetworkTransmitErrorsTotal,
	}

	cms := make([]*containerMetric, 0, len(networkDescs))
	for _, desc := range networkDescs {
		cms = append(cms, &containerMetric{
			desc: desc,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      1,
					labels:     []string{"eth0"},
					metricType: types.MetricType_COUNTER,
				}}
			},
		})
	}

	return computeMetrics(sandboxBaseLabels, cms)
}

func testBlkioStats() cgroups.BlkioStats {
	return cgroups.BlkioStats{
		IoServicedRecursive: []cgroups.BlkioStatEntry{
			{Major: 8, Minor: 0, Op: "Read", Value: 100},
			{Major: 8, Minor: 0, Op: "Write", Value: 200},
		},
		IoServiceBytesRecursive: []cgroups.BlkioStatEntry{
			{Major: 8, Minor: 0, Op: "Read", Value: 4096},
			{Major: 8, Minor: 0, Op: "Write", Value: 8192},
		},
		PSI: &cgroups.PSIStats{
			Full: cgroups.PSIData{Total: 300000},
			Some: cgroups.PSIData{Total: 600000},
		},
	}
}
