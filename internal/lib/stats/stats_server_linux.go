package statsserver

import (
	"context"
	"errors"
	"fmt"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// updateSandbox updates the StatsServer's entry for this sandbox, as well as each child container.
// It first populates the stats from the CgroupParent, then calculates network usage, updates
// each of its children container stats by calling into the runtime, and finally calculates the CPUNanoCores.
func (ss *StatsServer) updateSandbox(sb *sandbox.Sandbox) *types.PodSandboxStats {
	if sb == nil {
		return nil
	}
	sandboxStats := &types.PodSandboxStats{
		Attributes: &types.PodSandboxAttributes{
			Id:          sb.ID(),
			Labels:      sb.Labels(),
			Metadata:    sb.Metadata(),
			Annotations: sb.Annotations(),
		},
		Linux: &types.LinuxPodSandboxStats{},
	}
	if err := ss.Config().CgroupManager().PopulateSandboxCgroupStats(sb.CgroupParent(), sandboxStats); err != nil {
		logrus.Errorf("Error getting sandbox stats %s: %v", sb.ID(), err)
	}
	if err := ss.populateNetworkUsage(sandboxStats, sb); err != nil {
		logrus.Errorf("Error adding network stats for sandbox %s: %v", sb.ID(), err)
	}
	containerStats := make([]*types.ContainerStats, 0, len(sb.Containers().List()))
	for _, c := range sb.Containers().List() {
		if c.StateNoLock().Status == oci.ContainerStateStopped {
			continue
		}
		cStats, err := ss.Runtime().ContainerStats(context.TODO(), c, sb.CgroupParent())
		if err != nil {
			logrus.Errorf("Error getting container stats %s: %v", c.ID(), err)
			continue
		}
		ss.populateWritableLayer(cStats, c)
		if oldcStats, ok := ss.ctrStats[c.ID()]; ok {
			updateUsageNanoCores(oldcStats.Cpu, cStats.Cpu)
		}
		containerStats = append(containerStats, cStats)
	}
	sandboxStats.Linux.Containers = containerStats
	if old, ok := ss.sboxStats[sb.ID()]; ok {
		updateUsageNanoCores(old.Linux.Cpu, sandboxStats.Linux.Cpu)
	}
	ss.sboxStats[sb.ID()] = sandboxStats
	return sandboxStats
}

// updateContainer calls into the runtime handler to update the container stats,
// as well as populates the writable layer by calling into the container storage.
// If this container already existed in the stats server, the CPU nano cores are calculated as well.
func (ss *StatsServer) updateContainer(c *oci.Container, sb *sandbox.Sandbox) *types.ContainerStats {
	if c == nil || sb == nil {
		return nil
	}
	if c.StateNoLock().Status == oci.ContainerStateStopped {
		return nil
	}
	cStats, err := ss.Runtime().ContainerStats(context.TODO(), c, sb.CgroupParent())
	if err != nil {
		logrus.Errorf("Error getting container stats %s: %v", c.ID(), err)
		return nil
	}
	ss.populateWritableLayer(cStats, c)
	if oldcStats, ok := ss.ctrStats[c.ID()]; ok {
		updateUsageNanoCores(oldcStats.Cpu, cStats.Cpu)
	}
	ss.ctrStats[c.ID()] = cStats
	return cStats
}

// populateNetworkUsage gathers information about the network from within the sandbox's network namespace.
func (ss *StatsServer) populateNetworkUsage(stats *types.PodSandboxStats, sb *sandbox.Sandbox) error {
	return ns.WithNetNSPath(sb.NetNsPath(), func(_ ns.NetNS) error {
		links, err := netlink.LinkList()
		if err != nil {
			logrus.Errorf("Unable to retrieve network namespace links: %v", err)
			return err
		}
		stats.Linux.Network = &types.NetworkUsage{
			Interfaces: make([]*types.NetworkInterfaceUsage, 0, len(links)-1),
		}
		for i := range links {
			iface, err := linkToInterface(links[i])
			if err != nil {
				logrus.Errorf("Failed to %v for pod %s", err, sb.ID())
				continue
			}
			// TODO FIXME or DefaultInterfaceName?
			if i == 0 {
				stats.Linux.Network.DefaultInterface = iface
			} else {
				stats.Linux.Network.Interfaces = append(stats.Linux.Network.Interfaces, iface)
			}
		}
		return nil
	})
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
