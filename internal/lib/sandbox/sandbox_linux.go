package sandbox

import (
	"context"
	"fmt"

	"github.com/cri-o/cri-o/internal/log"
	"golang.org/x/sys/unix"
)

// UnmountShm removes the shared memory mount for the sandbox and returns an
// error if any failure occurs.
func (s *Sandbox) UnmountShm(ctx context.Context) error {
	_, span := log.StartSpan(ctx)
	defer span.End()
	fp := s.ShmPath()
	if fp == DevShmPath {
		return nil
	}

	// try to unmount, ignoring "not mounted" (EINVAL) error and
	// "already unmounted" (ENOENT) error
	if err := unix.Unmount(fp, unix.MNT_DETACH); err != nil && err != unix.EINVAL && err != unix.ENOENT {
		return fmt.Errorf("unable to unmount %s: %w", fp, err)
	}

	return nil
}
