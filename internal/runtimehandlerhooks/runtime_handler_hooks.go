package runtimehandlerhooks

import (
	"context"
	"sync"

	"github.com/opencontainers/runtime-tools/generate"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/oci"
	libconfig "github.com/cri-o/cri-o/pkg/config"
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

// HooksRetriever allows retrieving the runtime hooks for a given sandbox.
type HooksRetriever struct {
	config               *libconfig.Config
	highPerformanceHooks RuntimeHandlerHooks
}
