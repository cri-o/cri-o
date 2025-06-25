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

// NewHooksRetriever returns a pointer to a new retriever.
func NewHooksRetriever(ctx context.Context, config *libconfig.Config) *HooksRetriever {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	rhh := &HooksRetriever{
		config:               config,
		highPerformanceHooks: nil,
	}

	return rhh
}

// Get always returns DefaultCPULoadBalanceHooks for non-linux architectures.
func (hr *HooksRetriever) Get(ctx context.Context, runtimeName string, sandboxAnnotations map[string]string) RuntimeHandlerHooks {
	return &DefaultCPULoadBalanceHooks{}
}

// RestoreIrqBalanceConfig restores irqbalance service with original banned cpu mask settings
func RestoreIrqBalanceConfig(ctx context.Context, irqBalanceConfigFile, irqBannedCPUConfigFile, irqSmpAffinityProcFile string) error {
	return nil
}
