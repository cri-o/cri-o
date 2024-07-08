package runtimehandlerhooks

import (
	"context"
	"strings"
	"sync"

	"github.com/cri-o/cri-o/internal/log"
	crioann "github.com/cri-o/cri-o/pkg/annotations"
	libconfig "github.com/cri-o/cri-o/pkg/config"
)

// GetRuntimeHandlerHooks returns RuntimeHandlerHooks implementation by the runtime handler name.
func GetRuntimeHandlerHooks(ctx context.Context, config *libconfig.Config, handler string, annotations map[string]string) (RuntimeHandlerHooks, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	if strings.Contains(handler, HighPerformance) {
		log.Warnf(ctx, "The usage of the handler %q without adding high-performance feature annotations under allowed_annotations will be deprecated under 1.21", HighPerformance)
		return &HighPerformanceHooks{irqBalanceConfigFile: config.IrqBalanceConfigFile, cpusetLock: sync.Mutex{}, sharedCPUs: config.SharedCPUSet}, nil
	}
	if highPerformanceAnnotationsSpecified(annotations) {
		log.Warnf(ctx, "The usage of the handler %q without adding high-performance feature annotations under allowed_annotations will be deprecated under 1.21", HighPerformance)
		return &HighPerformanceHooks{irqBalanceConfigFile: config.IrqBalanceConfigFile, cpusetLock: sync.Mutex{}, sharedCPUs: config.SharedCPUSet}, nil
	}
	if cpuLoadBalancingAllowed(config) {
		return &DefaultCPULoadBalanceHooks{}, nil
	}

	return nil, nil
}

func highPerformanceAnnotationsSpecified(annotations map[string]string) bool {
	for k := range annotations {
		if strings.HasPrefix(k, crioann.CPULoadBalancingAnnotation) ||
			strings.HasPrefix(k, crioann.CPUQuotaAnnotation) ||
			strings.HasPrefix(k, crioann.IRQLoadBalancingAnnotation) ||
			strings.HasPrefix(k, crioann.CPUCStatesAnnotation) ||
			strings.HasPrefix(k, crioann.CPUFreqGovernorAnnotation) ||
			strings.HasPrefix(k, crioann.CPUSharedAnnotation) {
			return true
		}
	}
	return false
}

func cpuLoadBalancingAllowed(config *libconfig.Config) bool {
	cpuLoadBalancingAllowedAnywhereOnce.Do(func() {
		for _, runtime := range config.Runtimes {
			for _, ann := range runtime.AllowedAnnotations {
				if ann == crioann.CPULoadBalancingAnnotation {
					cpuLoadBalancingAllowedAnywhere = true
				}
			}
		}
		for _, workload := range config.Workloads {
			for _, ann := range workload.AllowedAnnotations {
				if ann == crioann.CPULoadBalancingAnnotation {
					cpuLoadBalancingAllowedAnywhere = true
				}
			}
		}
	})
	return cpuLoadBalancingAllowedAnywhere
}
