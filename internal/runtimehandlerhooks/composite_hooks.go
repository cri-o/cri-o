package runtimehandlerhooks

import (
	"context"

	"github.com/opencontainers/runtime-tools/generate"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/oci"
)

// CompositeHooks chains multiple RuntimeHandlerHooks implementations.
// Each hook method is called in order; if any returns an error, execution stops.
//
// This exists because HooksRetriever.Get() returns a single RuntimeHandlerHooks.
// When a pod needs multiple hooks (e.g. a burstable pod on a high-performance
// runtime handler needs both HighPerformanceHooks and GomaxprocsHooks),
// CompositeHooks wraps them so Get() can still return one interface.
// When only one hook applies, Get() returns it directly without wrapping.
type CompositeHooks struct {
	hooks []RuntimeHandlerHooks
}

func (c *CompositeHooks) PreCreate(ctx context.Context, specgen *generate.Generator, s *sandbox.Sandbox, cont *oci.Container) error {
	for _, h := range c.hooks {
		if err := h.PreCreate(ctx, specgen, s, cont); err != nil {
			return err
		}
	}

	return nil
}

func (c *CompositeHooks) PreStart(ctx context.Context, cont *oci.Container, s *sandbox.Sandbox) error {
	for _, h := range c.hooks {
		if err := h.PreStart(ctx, cont, s); err != nil {
			return err
		}
	}

	return nil
}

func (c *CompositeHooks) PreStop(ctx context.Context, cont *oci.Container, s *sandbox.Sandbox) error {
	for _, h := range c.hooks {
		if err := h.PreStop(ctx, cont, s); err != nil {
			return err
		}
	}

	return nil
}

func (c *CompositeHooks) PostStop(ctx context.Context, cont *oci.Container, s *sandbox.Sandbox) error {
	for _, h := range c.hooks {
		if err := h.PostStop(ctx, cont, s); err != nil {
			return err
		}
	}

	return nil
}
