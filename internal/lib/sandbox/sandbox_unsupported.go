//go:build !linux
// +build !linux

package sandbox

import (
	"context"
)

// UnmountShm removes the shared memory mount for the sandbox and returns an
// error if any failure occurs.
func (s *Sandbox) UnmountShm(ctx context.Context) error {
	return nil
}
