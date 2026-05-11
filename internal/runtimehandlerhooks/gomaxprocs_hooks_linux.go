package runtimehandlerhooks

import (
	"context"
	"strconv"
	"strings"

	"github.com/opencontainers/runtime-tools/generate"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
	crioann "github.com/cri-o/cri-o/pkg/annotations/v2"
	"github.com/cri-o/cri-o/pkg/config"
)

// GomaxprocsHooks injects the GOMAXPROCS environment variable into containers
// for burstable and best-effort pods. The fallback value acts as a minimum floor;
// for burstable pods with a CPU request, GOMAXPROCS is auto-calculated from
// the request's CPU shares and only used if it exceeds the floor.
type GomaxprocsHooks struct {
	fallback  int64
	workloads config.Workloads
}

func (g *GomaxprocsHooks) PreCreate(ctx context.Context, specgen *generate.Generator, s *sandbox.Sandbox, c *oci.Container) error {
	log.Infof(ctx, "Run gomaxprocs runtime handler pre-create hook for the container %q", c.ID())

	annotations := s.Annotations()

	// Skip if pod has the skip annotation.
	skipAnnotation, _ := crioann.GetAnnotationValue(annotations, crioann.SkipGoMaxProcs)
	if skipAnnotation == "true" {
		log.Debugf(ctx, "Skipping GOMAXPROCS injection: %s annotation is set", crioann.SkipGoMaxProcs)

		return nil
	}

	// Skip workload-partitioned pods — their cpuset is managed by workload partitioning.
	if g.workloads.IsWorkloadPartitioned(annotations) {
		log.Debugf(ctx, "Skipping GOMAXPROCS injection: pod is workload-partitioned")

		return nil
	}

	cgroupParent := s.CgroupParent()

	// Only inject for burstable and best-effort pods.
	// Guaranteed pods get exclusive CPUs via CPU Manager.
	if !strings.Contains(cgroupParent, "burstable") && !strings.Contains(cgroupParent, "besteffort") {
		return nil
	}

	// Read CPU shares and quota from the OCI spec.
	var shares uint64

	var cpuQuota int64

	if specgen.Config.Linux != nil &&
		specgen.Config.Linux.Resources != nil &&
		specgen.Config.Linux.Resources.CPU != nil {
		if specgen.Config.Linux.Resources.CPU.Shares != nil {
			shares = *specgen.Config.Linux.Resources.CPU.Shares
		}

		if specgen.Config.Linux.Resources.CPU.Quota != nil {
			cpuQuota = *specgen.Config.Linux.Resources.CPU.Quota
		}
	}

	// Skip if the container has a CPU limit (quota > 0). Go 1.25+ auto-detects
	// GOMAXPROCS from the cgroup quota, so injecting would override that.
	if cpuQuota > 0 {
		log.Debugf(ctx, "Skipping GOMAXPROCS injection: container has CPU limit (quota=%d)", cpuQuota)

		return nil
	}

	maxProcs := calculateGOMAXPROCS(int64(shares), g.fallback)

	injectGOMAXPROCS(specgen, maxProcs)

	return nil
}

// No-op.
func (*GomaxprocsHooks) PreStart(context.Context, *oci.Container, *sandbox.Sandbox) error {
	return nil
}

// No-op.
func (*GomaxprocsHooks) PreStop(context.Context, *oci.Container, *sandbox.Sandbox) error {
	return nil
}

// No-op.
func (*GomaxprocsHooks) PostStop(context.Context, *oci.Container, *sandbox.Sandbox) error {
	return nil
}

// calculateGOMAXPROCS derives the GOMAXPROCS value from CPU shares and a floor.
// Kubelet sets shares = cpu_request_in_millicores * 1024 / 1000.
// We reverse that with ceil(shares / 1024) to get the CPU count.
// The floor is used when the calculated value is lower.
func calculateGOMAXPROCS(shares, fallbackMaxProcs int64) int64 {
	return max(max((shares+1023)/1024, 1), fallbackMaxProcs)
}

// injectGOMAXPROCS sets the GOMAXPROCS environment variable to the given value.
// Injection is skipped if GOMAXPROCS is already set in the OCI spec's process env
// (which already includes default_env values merged by setupContainerEnvironmentAndWorkdir).
func injectGOMAXPROCS(specgen *generate.Generator, maxProcs int64) {
	for _, env := range specgen.Config.Process.Env {
		if strings.HasPrefix(env, "GOMAXPROCS=") {
			return
		}
	}

	specgen.AddProcessEnv("GOMAXPROCS", strconv.FormatInt(maxProcs, 10))
}
