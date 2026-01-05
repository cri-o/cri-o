//go:build !linux

package oci

import (
	"context"
	"errors"

	"github.com/cri-o/cri-o/internal/lib/stats"
)

// CgroupStats is not supported on non-Linux platforms for VM runtimes.
func (r *runtimeVM) CgroupStats(ctx context.Context, c *Container, _ string) (*stats.CgroupStats, error) {
	return nil, errors.New("cgroup stats not supported on this platform")
}

// DiskStats is not supported on non-Linux platforms for VM runtimes.
func (r *runtimeVM) DiskStats(ctx context.Context, c *Container, _ string) (*stats.DiskStats, error) {
	return nil, errors.New("disk stats not supported on this platform")
}
