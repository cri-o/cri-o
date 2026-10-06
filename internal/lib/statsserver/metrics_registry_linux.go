package statsserver

import (
	"slices"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/lib/stats"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/pkg/config"
)

// metricSource identifies the stats a metric reads so that collecting those
// stats can be skipped when no enabled metric needs them.
type metricSource int

const (
	// sourceStandalone marks metrics generated without the collected cgroup or
	// disk stats: from the container spec, the cgroup manager, or the pod.
	sourceStandalone metricSource = iota
	sourceCgroupStats
	sourceDiskStats
)

// containerMetricContext carries everything a metric generator may need.
type containerMetricContext struct {
	ss          *StatsServer
	sb          *sandbox.Sandbox
	c           *oci.Container
	cgroupStats *stats.CgroupStats
	diskStats   *stats.DiskStats
}

// metricDefinition ties a metric to the stats it reads and the function that
// generates it.
type metricDefinition struct {
	source   metricSource
	generate func(containerMetricContext) []*types.Metric
}

// metricDefinitions is the single source of truth mapping each pod metric to its
// data source and generator. The source both decides whether the backing stats
// are collected (CgroupStatsEnabled / DiskStatsEnabled) and selects the generator,
// so when a metric's source matches the stats its generator reads, a
// sourceCgroupStats generator always receives a non-nil cgroupStats. That match is
// a per-entry convention checked by TestContainerMetricsFromContainerStatsNilSafety.
// Adding a metric here wires up collection and generation together, with no
// separate list to keep in sync.
var metricDefinitions = map[string]metricDefinition{
	config.CPUMetrics: {
		source: sourceCgroupStats,
		generate: func(mc containerMetricContext) []*types.Metric {
			return generateContainerCPUMetrics(mc.c, &mc.cgroupStats.CpuStats)
		},
	},
	config.HugetlbMetrics: {
		source: sourceCgroupStats,
		generate: func(mc containerMetricContext) []*types.Metric {
			return generateContainerHugetlbMetrics(mc.c, mc.cgroupStats.HugetlbStats)
		},
	},
	config.DiskMetrics: {
		source: sourceDiskStats,
		generate: func(mc containerMetricContext) []*types.Metric {
			if mc.diskStats == nil {
				return nil
			}

			return generateContainerDiskMetrics(mc.c, &mc.diskStats.Filesystem)
		},
	},
	config.DiskIOMetrics: {
		source: sourceCgroupStats,
		generate: func(mc containerMetricContext) []*types.Metric {
			return generateContainerDiskIOMetrics(mc.c, &mc.cgroupStats.BlkioStats)
		},
	},
	config.MemoryMetrics: {
		source: sourceCgroupStats,
		generate: func(mc containerMetricContext) []*types.Metric {
			return generateContainerMemoryMetrics(mc.c, &mc.cgroupStats.MemoryStats)
		},
	},
	config.MemoryExtraMetrics: {
		source: sourceCgroupStats,
		generate: func(mc containerMetricContext) []*types.Metric {
			return generateContainerMemoryExtraMetrics(mc.c, &mc.cgroupStats.MemoryStats)
		},
	},
	config.OOMMetrics: {
		source: sourceStandalone,
		generate: func(mc containerMetricContext) []*types.Metric {
			return mc.ss.generateOOMMetrics(mc.sb, mc.c)
		},
	},
	config.NetworkMetrics: {
		// Network metrics are collected at the pod level only.
		source:   sourceStandalone,
		generate: func(containerMetricContext) []*types.Metric { return nil },
	},
	config.ProcessMetrics: {
		source: sourceCgroupStats,
		generate: func(mc containerMetricContext) []*types.Metric {
			return generateContainerProcessMetrics(
				mc.c,
				&mc.cgroupStats.PidsStats,
				&mc.cgroupStats.ProcessStats,
			)
		},
	},
	config.SpecMetrics: {
		source: sourceStandalone,
		generate: func(mc containerMetricContext) []*types.Metric {
			return generateContainerSpecMetrics(mc.c)
		},
	},
	config.PressureMetrics: {
		source: sourceCgroupStats,
		generate: func(mc containerMetricContext) []*types.Metric {
			return generateContainerPressureMetrics(
				mc.c,
				&mc.cgroupStats.CpuStats,
				&mc.cgroupStats.MemoryStats,
				&mc.cgroupStats.BlkioStats,
			)
		},
	},
}

// CgroupStatsEnabled reports whether any of the enabled metrics require collecting
// cgroup stats from the runtime.
func CgroupStatsEnabled(enabledMetrics []string) bool {
	return anyMetricHasSource(enabledMetrics, sourceCgroupStats)
}

// DiskStatsEnabled reports whether the enabled metrics require collecting disk
// stats from the runtime.
func DiskStatsEnabled(enabledMetrics []string) bool {
	return anyMetricHasSource(enabledMetrics, sourceDiskStats)
}

func anyMetricHasSource(enabledMetrics []string, source metricSource) bool {
	return slices.ContainsFunc(enabledMetrics, func(m string) bool {
		def, ok := metricDefinitions[m]

		return ok && def.source == source
	})
}

func (ss *StatsServer) generateOOMMetrics(sb *sandbox.Sandbox, c *oci.Container) []*types.Metric {
	cm, err := ss.Config().CgroupManager().ContainerCgroupManager(sb.CgroupParent(), c.ID())
	if err != nil {
		log.Errorf(ss.ctx, "Unable to fetch cgroup manager for container %s: %v", c.ID(), err)

		return nil
	}

	oomCount, err := cm.OOMKillCount()
	if err != nil {
		log.Errorf(ss.ctx, "Unable to fetch OOM kill count for container %s: %v", c.ID(), err)

		return nil
	}

	return GenerateContainerOOMMetrics(c, oomCount)
}
