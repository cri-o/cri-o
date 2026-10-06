package statsserver

import (
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/lib/stats"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/pkg/config"
)

// containerMetricContext carries everything a metric generator may need.
type containerMetricContext struct {
	ss          *StatsServer
	sb          *sandbox.Sandbox
	c           *oci.Container
	cgroupStats *stats.CgroupStats
	diskStats   *stats.DiskStats
}

// metricDefinition ties a metric to the function that generates it.
type metricDefinition struct {
	generate func(containerMetricContext) []*types.Metric
}

// metricDefinitions is the single source of truth mapping each pod metric to its
// generator. Adding a metric here wires up its generation, with no separate switch
// to keep in sync.
var metricDefinitions = map[string]metricDefinition{
	config.CPUMetrics: {
		generate: func(mc containerMetricContext) []*types.Metric {
			return generateContainerCPUMetrics(mc.c, &mc.cgroupStats.CpuStats)
		},
	},
	config.HugetlbMetrics: {
		generate: func(mc containerMetricContext) []*types.Metric {
			return generateContainerHugetlbMetrics(mc.c, mc.cgroupStats.HugetlbStats)
		},
	},
	config.DiskMetrics: {
		generate: func(mc containerMetricContext) []*types.Metric {
			if mc.diskStats == nil {
				return nil
			}

			return generateContainerDiskMetrics(mc.c, &mc.diskStats.Filesystem)
		},
	},
	config.DiskIOMetrics: {
		generate: func(mc containerMetricContext) []*types.Metric {
			return generateContainerDiskIOMetrics(mc.c, &mc.cgroupStats.BlkioStats)
		},
	},
	config.MemoryMetrics: {
		generate: func(mc containerMetricContext) []*types.Metric {
			return generateContainerMemoryMetrics(mc.c, &mc.cgroupStats.MemoryStats)
		},
	},
	config.MemoryExtraMetrics: {
		generate: func(mc containerMetricContext) []*types.Metric {
			return generateContainerMemoryExtraMetrics(mc.c, &mc.cgroupStats.MemoryStats)
		},
	},
	config.OOMMetrics: {
		generate: func(mc containerMetricContext) []*types.Metric {
			return mc.ss.generateOOMMetrics(mc.sb, mc.c)
		},
	},
	config.NetworkMetrics: {
		// Network metrics are collected at the pod level only.
		generate: func(containerMetricContext) []*types.Metric { return nil },
	},
	config.ProcessMetrics: {
		generate: func(mc containerMetricContext) []*types.Metric {
			return generateContainerProcessMetrics(
				mc.c,
				&mc.cgroupStats.PidsStats,
				&mc.cgroupStats.ProcessStats,
			)
		},
	},
	config.SpecMetrics: {
		generate: func(mc containerMetricContext) []*types.Metric {
			return generateContainerSpecMetrics(mc.c)
		},
	},
	config.PressureMetrics: {
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
