package runtimehandlerhooks

import (
	"context"
	"strings"
	"sync"

	"github.com/cri-o/cri-o/internal/log"
	crioann "github.com/cri-o/cri-o/pkg/annotations/v2"
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

	for name, runtime := range config.RuntimeSnapshot().Runtimes {
		annotationMap := map[string]string{}
		for _, v := range runtime.AllowedAnnotations {
			annotationMap[v] = ""
		}

		if strings.Contains(name, HighPerformance) &&
			!highPerformanceAnnotationsSpecified(annotationMap) {
			log.Warnf(
				ctx,
				"The usage of the handler %q without adding high-performance feature annotations under "+
					"allowed_annotations is deprecated since 1.21",
				HighPerformance,
			)
		}
	}

	return rhh
}

// Get checks runtime name or the sandbox's annotations for allowed high performance annotations and
// the config for GOMAXPROCS injection. It returns a single hook, a CompositeHooks chain, or nil.
func (hr *HooksRetriever) Get(
	ctx context.Context,
	runtimeName string,
	sandboxAnnotations map[string]string,
) RuntimeHandlerHooks {
	var hooks []RuntimeHandlerHooks

	if strings.Contains(runtimeName, HighPerformance) ||
		highPerformanceAnnotationsSpecified(sandboxAnnotations) {
		runtimeConfig, ok := hr.config.RuntimeSnapshot().Runtimes[runtimeName]
		if !ok {
			// This shouldn't happen because runtime is already validated
			log.Errorf(ctx, "Config of runtime %s is not found", runtimeName)

			return nil
		}

		hr.highPerformanceHooksMutex.Lock()

		if hp, ok := hr.highPerformanceHooks.(*HighPerformanceHooks); !ok ||
			hp.execCPUAffinity != runtimeConfig.ExecCPUAffinity {
			// (Re)create the hooks, so that reloaded ExecCPUAffinity
			// configuration is applied to sandboxes created afterwards,
			// while already created sandboxes keep their hook instance.
			hr.highPerformanceHooks = &HighPerformanceHooks{
				CgroupManager:             hr.config.CgroupManager(),
				irqBalanceConfigFile:      hr.config.IrqBalanceConfigFile,
				cpusetLock:                sync.Mutex{},
				updateIRQSMPAffinityLock:  sync.Mutex{},
				irqSMPAffinityDisabledSet: map[string]struct{}{},
				sharedCPUs:                hr.config.SharedCPUSet,
				irqSMPAffinityFile:        IrqSmpAffinityProcFile,
				execCPUAffinity:           runtimeConfig.ExecCPUAffinity,
				sysCPUDir:                 sysCPUDir,
			}
		}

		hooks = append(hooks, hr.highPerformanceHooks)

		hr.highPerformanceHooksMutex.Unlock()
	} else if cpuLoadBalancingAllowed(
		hr.config,
	) {
		hooks = append(hooks, &DefaultCPULoadBalanceHooks{
			CgroupManager: hr.config.CgroupManager(),
		})
	}

	if hr.config.MinInjectedGOMAXPROCS > 0 {
		hooks = append(hooks, &GomaxprocsHooks{
			fallback: hr.config.MinInjectedGOMAXPROCS,
		})
	}

	switch len(hooks) {
	case 0:
		return nil
	case 1:
		return hooks[0]
	default:
		return &CompositeHooks{hooks: hooks}
	}
}

func highPerformanceAnnotationsSpecified(annotations map[string]string) bool {
	for k := range annotations {
		if strings.HasPrefix(k, crioann.CPULoadBalancing) ||
			strings.HasPrefix(k, crioann.CPUQuota) ||
			strings.HasPrefix(k, crioann.IRQLoadBalancing) ||
			strings.HasPrefix(k, crioann.CPUCStates) ||
			strings.HasPrefix(k, crioann.CPUFreqGovernor) ||
			strings.HasPrefix(k, crioann.CPUShared) {
			return true
		}
	}

	return false
}

func cpuLoadBalancingAllowed(config *libconfig.Config) bool {
	snapshot := config.RuntimeSnapshot()

	// Only recompute when the runtime configuration changed, so that
	// reloads are reflected by subsequent requests while concurrent
	// requests reuse the result computed for the same snapshot. Both
	// values are published as one atomic unit, so a result can never be
	// paired with a different snapshot than it was computed for.
	if cached := cpuLoadBalancingAllowedAnywhere.Load(); cached != nil &&
		cached.snapshot == snapshot {
		return cached.allowed
	}

	allowed := false

	for _, runtime := range snapshot.Runtimes {
		for _, ann := range runtime.AllowedAnnotations {
			if ann == crioann.CPULoadBalancing {
				allowed = true
			}
		}
	}

	for _, workload := range config.Workloads {
		for _, ann := range workload.AllowedAnnotations {
			if ann == crioann.CPULoadBalancing {
				allowed = true
			}
		}
	}

	cpuLoadBalancingAllowedAnywhere.Store(&cpuLoadBalancingCache{
		snapshot: snapshot,
		allowed:  allowed,
	})

	return allowed
}
