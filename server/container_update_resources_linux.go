//go:build linux

package server

import (
	"context"
	"fmt"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
)

// validateMemoryUpdate checks if the new memory limit is safe to apply by getting current usage from CRI-O's stats server.
// This ensures real-time accuracy and prevents decreasing memory limits below current usage.
func (s *Server) validateMemoryUpdate(ctx context.Context, c *oci.Container, newMemoryLimit int64) error {
	// Negative memory limits are invalid
	if newMemoryLimit < 0 {
		return fmt.Errorf("invalid memory limit: %d bytes (cannot be negative)", newMemoryLimit)
	}

	// Zero means unlimited memory, no validation needed.
	if newMemoryLimit == 0 {
		return nil
	}

	sb := s.GetSandbox(c.Sandbox())
	if sb == nil {
		log.Warnf(ctx, "Could not get sandbox %s for container %s", c.Sandbox(), c.ID())

		return nil
	}

	containerStats := s.StatsForContainer(c, sb)
	if containerStats == nil {
		log.Warnf(ctx, "No memory stats available for container %s", c.ID())

		return nil
	}

	usageBytes := containerStats.GetMemory().GetUsageBytes()
	if usageBytes == nil {
		log.Warnf(ctx, "Memory usage not available for container %s", c.ID())

		return nil
	}

	currentUsage := int64(usageBytes.GetValue())

	// Check if new limit is below current usage.
	if newMemoryLimit < currentUsage {
		return fmt.Errorf("cannot decrease memory limit to %d bytes: current usage is %d bytes",
			newMemoryLimit, currentUsage)
	}

	log.Debugf(ctx, "Memory validation passed: new limit %d >= current usage %d",
		newMemoryLimit, currentUsage)

	return nil
}
