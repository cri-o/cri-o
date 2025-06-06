package runtimehandlerhooks

import (
	"context"
	"sync"

	"github.com/opencontainers/runtime-tools/generate"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/oci"
)

var (
	cpuLoadBalancingAllowedAnywhereOnce sync.Once
	cpuLoadBalancingAllowedAnywhere     bool
)

//nolint:iface // interface duplication is intentional
type RuntimeHandlerHooks interface {
	PreCreate(ctx context.Context, specgen *generate.Generator, s *sandbox.Sandbox, c *oci.Container) error
	PreStart(ctx context.Context, c *oci.Container, s *sandbox.Sandbox) error
	PreStop(ctx context.Context, c *oci.Container, s *sandbox.Sandbox) error
	PostStop(ctx context.Context, c *oci.Container, s *sandbox.Sandbox) error
}

//nolint:iface // interface duplication is intentional
type HighPerformanceHook interface {
	RuntimeHandlerHooks
}

// Map holds a list of RuntimeHandlerHooks for each registered runtime handler.
type Map map[string]RuntimeHandlerHooks

// Get gets the registered runtime handler's hook or nil if none is found.
func (m Map) Get(name string) RuntimeHandlerHooks {
	if r, ok := m[name]; ok {
		return r
	}
	// Return nil to avoid the odd case where the runtime wasn't registered as we don't want to error.
	return nil
}
