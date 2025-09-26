package runtimehandlerhooks

import (
	"context"
	"strings"
	"sync"

	"github.com/cri-o/cri-o/internal/log"
	crioann "github.com/cri-o/cri-o/pkg/annotations"
	libconfig "github.com/cri-o/cri-o/pkg/config"
)

// NewHooksRetriever returns a pointer to a new retriever.
// Log a warning if deprecated configuration is detected.
func NewHooksRetriever(ctx context.Context, config *libconfig.Config) *HooksRetriever {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	rhh := &HooksRetriever{
		config:               config,
		highPerformanceHooks: nil,
	}

	for name, runtime := range config.Runtimes {
		annotationMap := map[string]string{}
		for _, v := range runtime.AllowedAnnotations {
			annotationMap[v] = ""
		}

		if strings.Contains(name, HighPerformance) && !highPerformanceAnnotationsSpecified(annotationMap) {
			log.Warnf(ctx, "The usage of the handler %q without adding high-performance feature annotations under "+
				"allowed_annotations is deprecated since 1.21", HighPerformance)
		}
	}

	return rhh
}

// Get checks runtime name or the sandbox's annotations for allowed high performance annotations. If present, it returns
// the single instance of highPerformanceHooks.
// Otherwise, if crio's config allows CPU load balancing anywhere, return a DefaultCPULoadBalanceHooks.
// Otherwise, return nil.
func (hr *HooksRetriever) Get(ctx context.Context, runtimeName string, sandboxAnnotations map[string]string) RuntimeHandlerHooks {
	if strings.Contains(runtimeName, HighPerformance) || highPerformanceAnnotationsSpecified(sandboxAnnotations) {
		runtimeConfig, ok := hr.config.Runtimes[runtimeName]
		if !ok {
			// This shouldn't happen because runtime is already validated
			log.Errorf(ctx, "Config of runtime %s is not found", runtimeName)

			return nil
		}

		if hr.highPerformanceHooks == nil {
			hr.highPerformanceHooks = &HighPerformanceHooks{
				irqBalanceConfigFile:     hr.config.IrqBalanceConfigFile,
				cpusetLock:               sync.Mutex{},
				updateIRQSMPAffinityLock: sync.Mutex{},
				sharedCPUs:               hr.config.SharedCPUSet,
				irqSMPAffinityFile:       IrqSmpAffinityProcFile,
				execCPUAffinity:          runtimeConfig.ExecCPUAffinity,
			}
		}

		return hr.highPerformanceHooks
	}

	if cpuLoadBalancingAllowed(hr.config) {
		return &DefaultCPULoadBalanceHooks{}
	}

	return nil
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
