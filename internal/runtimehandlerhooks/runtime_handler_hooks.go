package runtimehandlerhooks

import (
	"context"
	"strings"
	"sync"

	"github.com/opencontainers/runtime-tools/generate"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
	libconfig "github.com/cri-o/cri-o/pkg/config"
)

var (
	cpuLoadBalancingAllowedAnywhereOnce sync.Once
	cpuLoadBalancingAllowedAnywhere     bool
)

//nolint:iface // interface duplication is intentional
type RuntimeHandlerHooks interface {
	PreCreate(ctx context.Context, specgen *generate.Generator, s *sandbox.Sandbox, c *oci.Container) error
	PreStart(ctx context.Context, c *oci.Container, s *sandbox.Sandbox) error
	PreStop(ctx context.Context, c *oci.Container, s *sandbox.Sandbox) error
	PostStop(ctx context.Context, c *oci.Container, s *sandbox.Sandbox) error
}

//nolint:iface // interface duplication is intentional
type HighPerformanceHook interface {
	RuntimeHandlerHooks
}

// Map holds a list of RuntimeHandlerHooks for each registered runtime handler.
type Map map[string]RuntimeHandlerHooks

// Get gets the registered runtime handler's hook or nil if none is found.
func (m Map) Get(name string) RuntimeHandlerHooks {
	if r, ok := m[name]; ok {
		return r
	}
	// Return nil to avoid the odd case where the runtime wasn't registered as we don't want to error.
	return nil
}

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
