package runtimehandlerhooks

import (
	"context"
	"strings"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
)

type RuntimeHandlerHooks interface {
	PreStart(ctx context.Context, c *oci.Container, s *sandbox.Sandbox) error
	PreStop(ctx context.Context, c *oci.Container, s *sandbox.Sandbox) error
}

// GetRuntimeHandlerHooks returns RuntimeHandlerHooks implementation by the runtime handler name
func GetRuntimeHandlerHooks(ctx context.Context, handler string, r *oci.Runtime) (RuntimeHandlerHooks, error) {
	allowAnnotations, err := allowHighPerformanceAnnotations(handler, r)
	if err != nil {
		return nil, err
	}

	if allowAnnotations {
		return &HighPerformanceHooks{}, nil
	}

	if !allowAnnotations && strings.Contains(handler, HighPerformance) {
		log.Warnf(ctx, "The usage of the handler %q without adding high-performance feature annotations under allowed_annotations will be deprecated under 1.21", HighPerformance)
		return &HighPerformanceHooks{}, nil
	}

	return nil, nil
}

func allowHighPerformanceAnnotations(handler string, r *oci.Runtime) (bool, error) {
	allowCPULoadBalancing, err := r.AllowCPUQuotaAnnotation(handler)
	if err != nil {
		return false, err
	}

	allowIRQLoadBalancing, err := r.AllowIRQLoadBalancingAnnotation(handler)
	if err != nil {
		return false, err
	}

	allowCPUQuota, err := r.AllowCPUQuotaAnnotation(handler)
	if err != nil {
		return false, err
	}

	return allowCPULoadBalancing || allowIRQLoadBalancing || allowCPUQuota, nil
}
