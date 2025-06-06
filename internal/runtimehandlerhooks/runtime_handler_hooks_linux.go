package runtimehandlerhooks

import (
	"context"
	"strings"
	"sync"

	"github.com/cri-o/cri-o/internal/log"
	crioann "github.com/cri-o/cri-o/pkg/annotations"
	libconfig "github.com/cri-o/cri-o/pkg/config"
)

// NewMap creates a new Map of runtime names to runtime hooks from Crio's configuration.
func NewMap(ctx context.Context, config *libconfig.Config) Map {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	var hphInstance *HighPerformanceHooks

	rhh := make(Map)

	for name, runtime := range config.Runtimes {
		if strings.Contains(name, HighPerformance) && !highPerformanceAnnotationsSpecified(runtime.AllowedAnnotations) {
			log.Warnf(ctx, "The usage of the handler %q without adding high-performance feature annotations under "+
				"allowed_annotations is deprecated since 1.21", HighPerformance)
		}

		if highPerformanceAnnotationsSpecified(runtime.AllowedAnnotations) || strings.Contains(name, HighPerformance) {
			if hphInstance == nil {
				hphInstance = &HighPerformanceHooks{
					irqBalanceConfigFile:     config.IrqBalanceConfigFile,
					cpusetLock:               sync.Mutex{},
					irqSMPAffinityFileLock:   sync.Mutex{},
					irqBalanceConfigFileLock: sync.Mutex{},
					sharedCPUs:               config.SharedCPUSet,
					irqSMPAffinityFile:       IrqSmpAffinityProcFile,
				}
			}

			rhh[name] = hphInstance

			continue
		}

		if cpuLoadBalancingAllowed(config) {
			rhh[name] = &DefaultCPULoadBalanceHooks{}

			continue
		}

		rhh[name] = nil
	}

	return rhh
}

func highPerformanceAnnotationsSpecified(annotations []string) bool {
	for _, k := range annotations {
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
