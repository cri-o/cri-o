//go:build linux

package oci

import (
	"context"
	"testing"

	cgroupsV1 "github.com/containerd/cgroups/stats/v1"
	cgroupsV2 "github.com/containerd/cgroups/v2/stats"
)

// TestMetricsV2ToCgroupStatsSwapOnly verifies that the cgroup v2 VM stats
// converter records the guest's swap-only usage in SwapOnlyUsage. cgroup v2
// reports swap without memory, so it is the swap-only value directly.
func TestMetricsV2ToCgroupStatsSwapOnly(t *testing.T) {
	t.Parallel()

	m := &cgroupsV2.Metrics{
		CPU: &cgroupsV2.CPUStat{},
		Memory: &cgroupsV2.MemoryStat{
			Usage:     500,
			SwapUsage: 100,
		},
		Pids: &cgroupsV2.PidsStat{},
	}

	cgStats := metricsV2ToCgroupStats(context.Background(), m)

	if got, want := cgStats.MemoryStats.SwapOnlyUsage.Usage, uint64(100); got != want {
		t.Errorf("SwapOnlyUsage.Usage: got %d, want %d", got, want)
	}
}

// TestMetricsV1ToCgroupStatsSwapOnly verifies that the cgroup v1 VM stats
// converter recovers swap-only usage by subtracting memory from the combined
// memsw value the guest reports.
func TestMetricsV1ToCgroupStatsSwapOnly(t *testing.T) {
	t.Parallel()

	m := &cgroupsV1.Metrics{
		CPU: &cgroupsV1.CPUStat{
			Usage:      &cgroupsV1.CPUUsage{},
			Throttling: &cgroupsV1.Throttle{},
		},
		Memory: &cgroupsV1.MemoryStat{
			Usage:     &cgroupsV1.MemoryEntry{Usage: 500},
			Swap:      &cgroupsV1.MemoryEntry{Usage: 600},
			Kernel:    &cgroupsV1.MemoryEntry{},
			KernelTCP: &cgroupsV1.MemoryEntry{},
		},
		Pids: &cgroupsV1.PidsStat{},
	}

	cgStats := metricsV1ToCgroupStats(context.Background(), m)

	if got, want := cgStats.MemoryStats.SwapOnlyUsage.Usage, uint64(100); got != want {
		t.Errorf("SwapOnlyUsage.Usage: got %d, want %d (memsw 600 - memory 500)", got, want)
	}
}
