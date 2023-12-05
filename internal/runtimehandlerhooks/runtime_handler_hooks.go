package runtimehandlerhooks

import (
	"context"
	"sync"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/oci"
)

var (
	cpuLoadBalancingAllowedAnywhereOnce sync.Once
	cpuLoadBalancingAllowedAnywhere     bool
)

type RuntimeHandlerHooks interface {
	PreStart(ctx context.Context, c *oci.Container, s *sandbox.Sandbox) error
	PreStop(ctx context.Context, c *oci.Container, s *sandbox.Sandbox) error
	PostStop(ctx context.Context, c *oci.Container, s *sandbox.Sandbox) error
}
