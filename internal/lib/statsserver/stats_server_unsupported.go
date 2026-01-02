//go:build !linux

package statsserver

import (
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/oci"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// updateSandbox updates the StatsServer's entry for this sandbox, as well as each child container.
// It first populates the stats from the CgroupParent, then calculates network usage, updates
// each of its children container stats by calling into the runtime, and finally calculates the CPUNanoCores.
func (ss *StatsServer) updateSandbox(sb *sandbox.Sandbox) *types.PodSandboxStats {
	return &types.PodSandboxStats{}
}

// updateContainer calls into the runtime handler to update the container stats,
// as well as populates the writable layer by calling into the container storage.
// If this container already existed in the stats server, the CPU nano cores are calculated as well.
func (ss *StatsServer) updateContainerStats(c *oci.Container, sb *sandbox.Sandbox) *types.ContainerStats {
	return &types.ContainerStats{}
}

// metricsForPodSandbox is an internal, non-locking version of MetricsForPodSandbox
// that returns (and occasionally gathers) the metrics for the given sandbox.
// Note: the caller must hold the lock on the StatsServer
func (ss *StatsServer) metricsForPodSandbox(sb *sandbox.Sandbox) *SandboxMetrics {
	return &SandboxMetrics{}
}
