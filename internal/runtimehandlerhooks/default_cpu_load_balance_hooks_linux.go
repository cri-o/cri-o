package runtimehandlerhooks

import (
	"context"
	"errors"

	"github.com/opencontainers/runtime-tools/generate"

	"github.com/cri-o/cri-o/internal/config/cgmgr"
	"github.com/cri-o/cri-o/internal/config/node"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/oci"
)

// DefaultCPULoadBalanceHooks is used to run additional hooks that will configure containers for CPU load balancing.
// Specifically, it will define a PostStop that disables `cpuset.sched_load_balance` for a recently stopped container.
// This must be done because guaranteed pods with exclusive cpu access may be created after other containers are terminated,
// but before their cgroup is cleaned up. In this case, cpumanager will not load balancing the exclusive CPUs away from those pods,
// thus causing their `cpuset.sched_load_balance=1` to prevent the kernel from disabling load balancing.
// This is the only case it seeks to fix, and thus does not define any other members of the RuntimeHandlerHooks functions.
type DefaultCPULoadBalanceHooks struct {
	cgmgr.CgroupManager
}

// No-op.
func (*DefaultCPULoadBalanceHooks) PreCreate(context.Context, *generate.Generator, *sandbox.Sandbox, *oci.Container) error {
	return nil
}

// No-op.
func (*DefaultCPULoadBalanceHooks) PreStart(context.Context, *oci.Container, *sandbox.Sandbox) error {
	return nil
}

// No-op.
func (*DefaultCPULoadBalanceHooks) PreStop(context.Context, *oci.Container, *sandbox.Sandbox) error {
	return nil
}

func (d *DefaultCPULoadBalanceHooks) PostStop(ctx context.Context, c *oci.Container, s *sandbox.Sandbox) error {
	// Disable cpuset.sched_load_balance for all stale cgroups.
	// This way, cpumanager can ignore stopped containers, but the running ones will still have exclusive access.
	if c.Spoofed() || node.CgroupIsV2() {
		return nil
	}

	if d.CgroupManager == nil {
		return errors.New("cgroup manager is nil")
	}

	_, containerManagers, err := d.PodAndContainerCgroupManagers(s.CgroupParent(), c.ID())
	if err != nil {
		return err
	}

	return disableCPULoadBalancingV1(containerManagers)
}
