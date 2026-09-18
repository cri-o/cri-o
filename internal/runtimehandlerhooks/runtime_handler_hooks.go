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

var (
	// cpuLoadBalancingAllowedAnywhereSnapshot caches the runtime snapshot
	// the cpuLoadBalancingAllowedAnywhere result was computed for, so that
	// the result is recomputed when a reload publishes a new configuration.
	cpuLoadBalancingAllowedAnywhereSnapshot atomic.Pointer[libconfig.RuntimeSnapshot]
	cpuLoadBalancingAllowedAnywhere         atomic.Bool
)

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
