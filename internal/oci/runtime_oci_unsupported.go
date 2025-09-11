//go:build !linux

package oci

import (
	"context"
	"fmt"
)

// getContainerDiskStats is not supported on this platform.
func (r *runtimeOCI) getContainerFileDescriptors(ctx context.Context, c *Container) (uint64, error) {
	return 0, fmt.Errorf("file descriptors collection not supported on this platform")
}
