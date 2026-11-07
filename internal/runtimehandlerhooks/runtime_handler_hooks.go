package runtimehandlerhooks

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/opencontainers/runtime-tools/generate"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/oci"
	libconfig "github.com/cri-o/cri-o/pkg/config"
)

// cpuLoadBalancingCache caches the CPU load-balancing permission together
// with the runtime snapshot it was computed for, so that the result is
// recomputed when a reload publishes a new configuration, and so that both
// values are always published as a single unit.
type cpuLoadBalancingCache struct {
	snapshot *libconfig.RuntimeSnapshot
	allowed  bool
}

var cpuLoadBalancingAllowedAnywhere atomic.Pointer[cpuLoadBalancingCache]

//nolint:iface // interface duplication is intentional
type RuntimeHandlerHooks interface {
	PreCreate(
		ctx context.Context,
		specgen *generate.Generator,
		s *sandbox.Sandbox,
		c *oci.Container,
	) error
	PreStart(ctx context.Context, c *oci.Container, s *sandbox.Sandbox) error
	PreStop(ctx context.Context, c *oci.Container, s *sandbox.Sandbox) error
	PostStop(ctx context.Context, c *oci.Container, s *sandbox.Sandbox) error
}

//nolint:iface // interface duplication is intentional
type HighPerformanceHook interface {
	RuntimeHandlerHooks
}

// HooksRetriever allows retrieving the runtime hooks for a given sandbox.
type HooksRetriever struct {
	config                    *libconfig.Config
	highPerformanceHooks      RuntimeHandlerHooks
	highPerformanceHooksMutex sync.Mutex
}
