package runtimehandlerhooks

import (
	"context"
	"sync"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/opencontainers/runtime-tools/generate"
)

var (
	cpuLoadBalancingAllowedAnywhereOnce sync.Once
	cpuLoadBalancingAllowedAnywhere     bool
)

type RuntimeHandlerHooks interface {
	PreCreate(ctx context.Context, specgen *generate.Generator, s *sandbox.Sandbox, c *oci.Container) error
	PreStart(ctx context.Context, c *oci.Container, s *sandbox.Sandbox) error
	PreStop(ctx context.Context, c *oci.Container, s *sandbox.Sandbox) error
	PostStop(ctx context.Context, c *oci.Container, s *sandbox.Sandbox) error
}

type HighPerformanceHook interface {
	RuntimeHandlerHooks
}
