package statsserver

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/opencontainers/cgroups"
	"github.com/vishvananda/netlink"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/config/node"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/lib/stats"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/pkg/config"
)

// updateSandbox updates the StatsServer's entry for this sandbox, as well as each child container.
// It first populates the stats from the CgroupParent, then calculates network usage, updates
// each of its children container stats by calling into the runtime, and finally calculates the CPUNanoCores.
func (ss *StatsServer) updateSandbox(sb *sandbox.Sandbox) *types.PodSandboxStats {
	if sb == nil {
		return nil
	}

	// Sandbox metrics are to fulfill the CRI metrics endpoint.
	sandboxMetrics, exists := ss.sboxMetrics[sb.ID()]
	if !exists {
		sandboxMetrics = NewSandboxMetrics(sb)
	}

	// Sandbox stats are to fulfill the Kubelet's /stats/summary endpoint.
	sandboxStats := &types.PodSandboxStats{
		Attributes: &types.PodSandboxAttributes{
			Id:          sb.ID(),
			Labels:      sb.Labels(),
			Metadata:    sb.Metadata(),
			Annotations: sb.Annotations(),
		},
		Linux: &types.LinuxPodSandboxStats{},
	}

	// Network metrics are collected at pod level only.
	if slices.Contains(ss.Config().IncludedPodMetrics, config.NetworkMetrics) {
		podMetrics := ss.GenerateNetworkMetrics(sb)
		sandboxMetrics.metric.Metrics = podMetrics
	}

	if cgstats, err := ss.Config().CgroupManager().SandboxCgroupStats(sb.CgroupParent(), sb.ID()); err != nil {
		log.Errorf(ss.ctx, "Error getting sandbox stats %s: %v", sb.ID(), err)
	} else {
		sandboxStats.Linux.Cpu = criCPUStats(&cgstats.CpuStats, cgstats.SystemNano)
		sandboxStats.Linux.Memory = criMemStats(&cgstats.MemoryStats, cgstats.SystemNano)
		sandboxStats.Linux.Process = criProcessStats(&cgstats.PidsStats, cgstats.SystemNano)
	}

	if err := ss.populateNetworkUsage(sandboxStats, sb); err != nil {
		log.Errorf(ss.ctx, "Error adding network stats for sandbox %s: %v", sb.ID(), err)
	}

	containersList := sb.Containers().List()
	containerStats := make([]*types.ContainerStats, 0, len(containersList))
	containerMetrics := make([]*types.ContainerMetrics, 0, len(containersList))

	for _, c := range containersList {
		if c.StateNoLock().Status == oci.ContainerStateStopped {
			continue
		}

		ctrStats, err := ss.Runtime().ContainerStats(ss.ctx, c, sb.CgroupParent())
		if err != nil {
			log.Errorf(ss.ctx, "Error getting container stats %s: %v", c.ID(), err)
		}

		diskStats, err := ss.Runtime().DiskStats(ss.ctx, c, sb.CgroupParent())
		if err != nil {
			log.Errorf(ss.ctx, "Error getting disk stats %s: %v", c.ID(), err)
		}
		// Convert container stats (cgroup + disk) to CRI stats.
		cStats := containerCRIStats(ctrStats, diskStats, c, ctrStats.SystemNano)
		ss.populateWritableLayer(cStats, c)

		if oldcStats, ok := ss.ctrStats[c.ID()]; ok {
			updateUsageNanoCores(oldcStats.GetCpu(), cStats.GetCpu())
		}

		containerStats = append(containerStats, cStats)

		// Convert cgroups stats to CRI metrics.
		cMetrics := ss.containerMetricsFromContainerStats(sb, c, ctrStats, diskStats)
		containerMetrics = append(containerMetrics, cMetrics)
	}

	sandboxStats.Linux.Containers = containerStats
	sandboxMetrics.metric.ContainerMetrics = containerMetrics

	if old, ok := ss.sboxStats[sb.ID()]; ok {
		updateUsageNanoCores(old.GetLinux().GetCpu(), sandboxStats.GetLinux().GetCpu())
	}

	ss.sboxStats[sb.ID()] = sandboxStats
	ss.sboxMetrics[sb.ID()] = sandboxMetrics

	return sandboxStats
}

// updateContainerStats calls into the runtime handler to update the container stats,
// as well as populates the writable layer by calling into the container storage.
// If this container already existed in the stats server, the CPU nano cores are calculated as well.
func (ss *StatsServer) updateContainerStats(c *oci.Container, sb *sandbox.Sandbox) *types.ContainerStats {
	if c == nil || sb == nil {
		return nil
	}

	if c.StateNoLock().Status == oci.ContainerStateStopped {
		return nil
	}

	ctrStats, err := ss.Runtime().ContainerStats(ss.ctx, c, sb.CgroupParent())
	if err != nil {
		log.Errorf(ss.ctx, "Error getting container stats %s: %v", c.ID(), err)

		return nil
	}

	diskStats, err := ss.Runtime().DiskStats(ss.ctx, c, sb.CgroupParent())
	if err != nil {
		log.Errorf(ss.ctx, "Error getting disk stats %s: %v", c.ID(), err)
		// Continue without disk stats
		diskStats = nil
	}

	cStats := containerCRIStats(ctrStats, diskStats, c, ctrStats.SystemNano)
	ss.populateWritableLayer(cStats, c)

	if oldcStats, ok := ss.ctrStats[c.ID()]; ok {
		updateUsageNanoCores(oldcStats.GetCpu(), cStats.GetCpu())
	}

	ss.ctrStats[c.ID()] = cStats

	return cStats
}

// populateNetworkUsage gathers information about the network from within the sandbox's network namespace.
func (ss *StatsServer) populateNetworkUsage(sbStats *types.PodSandboxStats, sb *sandbox.Sandbox) error {
	return ns.WithNetNSPath(sb.NetNsPath(), func(_ ns.NetNS) error {
		links, err := netlink.LinkList()
		if err != nil {
			log.Errorf(ss.ctx, "Unable to retrieve network namespace links: %v", err)

			return err
		}

		sbStats.Linux.Network = &types.NetworkUsage{
			Interfaces: make([]*types.NetworkInterfaceUsage, 0, len(links)-1),
		}

		for i := range links {
			iface, err := linkToInterface(links[i])
			if err != nil {
				log.Errorf(ss.ctx, "Failed to %v for pod %s", err, sb.ID())

				continue
			}
			// TODO FIXME or DefaultInterfaceName?
			if i == 0 {
				sbStats.Linux.Network.DefaultInterface = iface
			} else {
				sbStats.Linux.Network.Interfaces = append(sbStats.Linux.Network.Interfaces, iface)
			}
		}

		return nil
	})
}

// metricsForPodSandbox is an internal, non-locking version of MetricsForPodSandbox
// that returns (and occasionally gathers) the metrics for the given sandbox.
// Note: caller must hold the lock on the StatsServer.
func (ss *StatsServer) metricsForPodSandbox(sb *sandbox.Sandbox) *SandboxMetrics {
	if ss.collectionPeriod == 0 {
		return ss.updatePodSandboxMetrics(sb)
	}

	if sboxMetrics, ok := ss.sboxMetrics[sb.ID()]; ok {
		return sboxMetrics
	}
	// Cache miss, try again.
	return ss.updatePodSandboxMetrics(sb)
}

// updatePodSandboxMetrics updates the sandbox metrics for the given sandbox.
// If the sandbox is not found, it creates a new entry in the map.
// Note: caller must hold the lock on the StatsServer.
func (ss *StatsServer) updatePodSandboxMetrics(sb *sandbox.Sandbox) *SandboxMetrics {
	if sb == nil {
		return nil
	}

	sm, exists := ss.sboxMetrics[sb.ID()]
	if !exists {
		sm = NewSandboxMetrics(sb)
	}
	// Network metrics are collected at the pod level.
	if slices.Contains(ss.Config().IncludedPodMetrics, config.NetworkMetrics) {
		podMetrics := ss.GenerateNetworkMetrics(sb)
		sm.metric.Metrics = podMetrics
	}

	containersList := sb.Containers().List()
	containerMetrics := make([]*types.ContainerMetrics, 0, len(containersList))

	for _, c := range containersList {
		// Skip if the container is stopped.
		if c.StateNoLock().Status == oci.ContainerStateStopped {
			continue
		}

		cMetrics := ss.GenerateSandboxContainerMetrics(sb, c, sm)
		containerMetrics = append(containerMetrics, cMetrics)
	}

	sm.metric.ContainerMetrics = containerMetrics
	ss.sboxMetrics[sb.ID()] = sm

	return sm
}

// GenerateSandboxContainerMetrics generates a list of metrics for the specified sandbox
// containers by collecting metrics from the cgroup based on the included pod metrics,
// except for network metrics, which are collected at the pod level.
func (ss *StatsServer) GenerateSandboxContainerMetrics(sb *sandbox.Sandbox, c *oci.Container, sm *SandboxMetrics) *types.ContainerMetrics {
	ctrStats, err := ss.Runtime().ContainerStats(ss.ctx, c, sb.CgroupParent())
	if err != nil || ctrStats == nil {
		log.Errorf(ss.ctx, "Error getting sandbox stats %s: %v", sb.ID(), err)

		return nil
	}

	diskStats, err := ss.Runtime().DiskStats(ss.ctx, c, sb.CgroupParent())
	if err != nil {
		log.Errorf(ss.ctx, "Error getting disk stats %s: %v", c.ID(), err)

		return nil
	}

	return ss.containerMetricsFromContainerStats(sb, c, ctrStats, diskStats)
}

func (ss *StatsServer) containerMetricsFromContainerStats(sb *sandbox.Sandbox, c *oci.Container, cgroupStats *stats.CgroupStats, diskStats *stats.DiskStats) *types.ContainerMetrics {
	metrics := computeContainerMetrics(c, []*containerMetric{{
		desc: containerLastSeen,
		valueFunc: func() metricValues {
			return metricValues{{
				value:      uint64(time.Now().Unix()),
				metricType: types.MetricType_GAUGE,
			}}
		},
	}}, "")

	for _, m := range ss.Config().IncludedPodMetrics {
		switch m {
		case config.CPUMetrics:
			if cpuMetrics := generateContainerCPUMetrics(c, &cgroupStats.CpuStats); cpuMetrics != nil {
				metrics = append(metrics, cpuMetrics...)
			}
		case config.HugetlbMetrics:
			if hugetlbMetrics := generateContainerHugetlbMetrics(c, cgroupStats.HugetlbStats); hugetlbMetrics != nil {
				metrics = append(metrics, hugetlbMetrics...)
			}
		case config.DiskMetrics:
			if diskMetrics := generateContainerDiskMetrics(c, &diskStats.Filesystem); diskMetrics != nil {
				metrics = append(metrics, diskMetrics...)
			}
		case config.DiskIOMetrics:
			if diskIOMetrics := generateContainerDiskIOMetrics(c, &cgroupStats.BlkioStats); diskIOMetrics != nil {
				metrics = append(metrics, diskIOMetrics...)
			}
		case config.MemoryMetrics:
			if memoryMetrics := generateContainerMemoryMetrics(c, &cgroupStats.MemoryStats); memoryMetrics != nil {
				metrics = append(metrics, memoryMetrics...)
			}
		case config.OOMMetrics:
			cm, err := ss.Config().CgroupManager().ContainerCgroupManager(sb.CgroupParent(), c.ID())
			if err != nil {
				log.Errorf(ss.ctx, "Unable to fetch cgroup manager for container %s: %v", c.ID(), err)

				continue
			}

			oomCount, err := cm.OOMKillCount()
			if err != nil {
				log.Errorf(ss.ctx, "Unable to fetch OOM kill count for container %s: %v", c.ID(), err)

				continue
			}

			oomMetrics := GenerateContainerOOMMetrics(c, oomCount)
			metrics = append(metrics, oomMetrics...)
		case config.NetworkMetrics:
			continue // Network metrics are collected at the pod level only.
		case config.ProcessMetrics:
			if processMetrics := generateContainerProcessMetrics(c, &cgroupStats.PidsStats, &cgroupStats.ProcessStats); processMetrics != nil {
				metrics = append(metrics, processMetrics...)
			}
		case config.SpecMetrics:
			if specMetrics := generateContainerSpecMetrics(c); specMetrics != nil {
				metrics = append(metrics, specMetrics...)
			}
		case config.PressureMetrics:
			if pressureMetrics := generateContainerPressureMetrics(c, &cgroupStats.CpuStats, &cgroupStats.MemoryStats, &cgroupStats.BlkioStats); pressureMetrics != nil {
				metrics = append(metrics, pressureMetrics...)
			}
		default:
			log.Warnf(ss.ctx, "Unknown metric: %s", m)
		}
	}

	return &types.ContainerMetrics{
		ContainerId: c.ID(),
		Metrics:     metrics,
	}
}

// linkToInterface translates information found from the netlink package
// into CRI the NetworkInterfaceUsage structure.
func linkToInterface(link netlink.Link) (*types.NetworkInterfaceUsage, error) {
	attrs := link.Attrs()
	if attrs == nil {
		return nil, errors.New("get stats for iface")
	}

	if attrs.Statistics == nil {
		return nil, fmt.Errorf("get stats for iface %s", attrs.Name)
	}

	return &types.NetworkInterfaceUsage{
		Name:     attrs.Name,
		RxBytes:  &types.UInt64Value{Value: attrs.Statistics.RxBytes},
		RxErrors: &types.UInt64Value{Value: attrs.Statistics.RxErrors},
		TxBytes:  &types.UInt64Value{Value: attrs.Statistics.TxBytes},
		TxErrors: &types.UInt64Value{Value: attrs.Statistics.TxErrors},
	}, nil
}

func containerCRIStats(cgstats *stats.CgroupStats, diskStats *stats.DiskStats, ctr *oci.Container, systemNano int64) *types.ContainerStats {
	criStats := &types.ContainerStats{
		Attributes: ctr.CRIAttributes(),
	}
	criStats.Cpu = criCPUStats(&cgstats.CpuStats, systemNano)
	criStats.Memory = criMemStats(&cgstats.MemoryStats, systemNano)
	criStats.Swap = criSwapStats(&cgstats.MemoryStats, systemNano)

	// Add filesystem stats if available
	if diskStats != nil {
		criStats.WritableLayer = criFilesystemStats(&diskStats.Filesystem, ctr, systemNano)
	}

	return criStats
}

func criCPUStats(cpuStats *cgroups.CpuStats, systemNano int64) *types.CpuUsage {
	return &types.CpuUsage{
		Timestamp:            systemNano,
		UsageCoreNanoSeconds: &types.UInt64Value{Value: cpuStats.CpuUsage.TotalUsage},
	}
}

func criMemStats(memStats *cgroups.MemoryStats, systemNano int64) *types.MemoryUsage {
	workingSetBytes, rssBytes, pageFaults, majorPageFaults, availableBytes := computeMemoryStats(memStats)

	return &types.MemoryUsage{
		Timestamp:       systemNano,
		WorkingSetBytes: &types.UInt64Value{Value: workingSetBytes},
		RssBytes:        &types.UInt64Value{Value: rssBytes},
		PageFaults:      &types.UInt64Value{Value: pageFaults},
		MajorPageFaults: &types.UInt64Value{Value: majorPageFaults},
		UsageBytes:      &types.UInt64Value{Value: memStats.Usage.Usage},
		AvailableBytes:  &types.UInt64Value{Value: availableBytes},
	}
}

func criSwapStats(memStats *cgroups.MemoryStats, systemNano int64) *types.SwapUsage {
	swapUsage := computeSwapUsage(memStats)

	return &types.SwapUsage{
		Timestamp:          systemNano,
		SwapUsageBytes:     &types.UInt64Value{Value: swapUsage},
		SwapAvailableBytes: &types.UInt64Value{Value: memStats.SwapUsage.Limit - swapUsage},
	}
}

func criProcessStats(pStats *cgroups.PidsStats, systemNano int64) *types.ProcessUsage {
	return &types.ProcessUsage{
		Timestamp:    systemNano,
		ProcessCount: &types.UInt64Value{Value: pStats.Current},
	}
}

func criFilesystemStats(diskStats *stats.FilesystemStats, ctr *oci.Container, systemNano int64) *types.FilesystemUsage {
	mountpoint := ctr.MountPoint()
	if mountpoint == "" {
		// Skip FS stats as mount point is unknown
		return nil
	}

	return &types.FilesystemUsage{
		Timestamp: systemNano,
		FsId:      &types.FilesystemIdentifier{Mountpoint: mountpoint},
		UsedBytes: &types.UInt64Value{Value: diskStats.UsageBytes},
	}
}

// computeMemoryStats computes derived memory statistics from cgroups.MemoryStats.
// Returns workingSetBytes, rssBytes, pageFaults, majorPageFaults, availableBytes.
func computeMemoryStats(memStats *cgroups.MemoryStats) (workingSetBytes, rssBytes, pageFaults, majorPageFaults, availableBytes uint64) {
	var inactiveFileName string

	usageBytes := memStats.Usage.Usage

	if node.CgroupIsV2() {
		// Use anon for rssBytes for cgroup v2 as in cAdvisor
		rssBytes = memStats.Stats["anon"]
		inactiveFileName = "inactive_file"
		pageFaults = memStats.Stats["pgfault"]
		majorPageFaults = memStats.Stats["pgmajfault"]
	} else {
		inactiveFileName = "total_inactive_file"
		rssBytes = memStats.Stats["total_rss"]
		// cgroup v1 doesn't have equivalent stats for pgfault and pgmajfault
	}

	workingSetBytes = usageBytes
	if v, ok := memStats.Stats[inactiveFileName]; ok {
		if workingSetBytes < v {
			workingSetBytes = 0
		} else {
			workingSetBytes -= v
		}
	}

	if !isMemoryUnlimited(memStats.Usage.Limit) {
		availableBytes = memStats.Usage.Limit - workingSetBytes
	}

	return workingSetBytes, rssBytes, pageFaults, majorPageFaults, availableBytes
}

// computeSwapUsage computes the actual swap usage from cgroups.MemoryStats.
func computeSwapUsage(memStats *cgroups.MemoryStats) uint64 {
	if node.CgroupIsV2() {
		// libcontainer adds memory.swap.current to memory.current and reports them as SwapUsage to be compatible with cgroup v1,
		// because cgroup v1 reports SwapUsage as mem+swap combined.
		// Here we subtract SwapUsage from memory usage to get the actual swap value.
		if memStats.SwapUsage.Usage > memStats.Usage.Usage {
			return memStats.SwapUsage.Usage - memStats.Usage.Usage
		}

		return 0
	}

	return memStats.SwapUsage.Usage
}

// isMemoryUnlimited checks if the memory limit is effectively unlimited.
func isMemoryUnlimited(v uint64) bool {
	return v == math.MaxUint64
}
