package runtimehandlerhooks

import (
	"context"
	"strings"

	"github.com/cri-o/cri-o/v1/lib/sandbox"
	"github.com/cri-o/cri-o/v1/oci"
)

type RuntimeHandlerHooks interface {
	PreStart(ctx context.Context, c *oci.Container, s *sandbox.Sandbox) error
	PreStop(ctx context.Context, c *oci.Container, s *sandbox.Sandbox) error
}

// GetRuntimeHandlerHooks returns RuntimeHandlerHooks implementation by the runtime handler name
func GetRuntimeHandlerHooks(handler string) RuntimeHandlerHooks {
	if strings.Contains(handler, HighPerformance) {
		return &HighPerformanceHooks{}
	}

	return nil
}
