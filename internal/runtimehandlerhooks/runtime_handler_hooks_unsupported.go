//go:build !linux

package runtimehandlerhooks

import (
	"context"

	"github.com/cri-o/cri-o/internal/log"
	libconfig "github.com/cri-o/cri-o/pkg/config"
)

const (
	IrqSmpAffinityProcFile = ""
)

// NewMap creates a new Map of runtime names to runtime hooks from Crio's configuration.
func NewMap(ctx context.Context, config *libconfig.Config) Map {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	rhh := make(Map)

	for name := range config.Runtimes {
		rhh[name] = nil
	}

	return rhh
}

// GetRuntimeHandlerHooks returns RuntimeHandlerHooks implementation by the runtime handler name
func GetRuntimeHandlerHooks(ctx context.Context, config *libconfig.Config, handler string, annotations map[string]string) (RuntimeHandlerHooks, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	return &DefaultCPULoadBalanceHooks{}, nil
}

// RestoreIrqBalanceConfig restores irqbalance service with original banned cpu mask settings
func RestoreIrqBalanceConfig(ctx context.Context, irqBalanceConfigFile, irqBannedCPUConfigFile, irqSmpAffinityProcFile string) error {
	return nil
}
