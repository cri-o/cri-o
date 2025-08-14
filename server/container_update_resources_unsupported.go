//go:build !linux

package server

import (
	"context"
	"fmt"

	"github.com/cri-o/cri-o/internal/oci"
)

// validateMemoryUpdate is a no-op on non-Linux platforms since cgroups don't exist.
// However, we still validate basic input constraints.
func (s *Server) validateMemoryUpdate(ctx context.Context, c *oci.Container, newMemoryLimit int64) error {
	// Negative memory limits are invalid
	if newMemoryLimit < 0 {
		return fmt.Errorf("invalid memory limit: %d bytes (cannot be negative)", newMemoryLimit)
	}

	return nil
}
