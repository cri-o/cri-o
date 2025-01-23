package sandbox

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/log"
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
	if err := unix.Unmount(fp, unix.MNT_DETACH); err != nil && !errors.Is(err, unix.EINVAL) && !errors.Is(err, unix.ENOENT) {
		return fmt.Errorf("unable to unmount %s: %w", fp, err)
	}

	return nil
}

// NeedsInfra is a function that returns whether the sandbox will need an infra container.
// If the server manages the namespace lifecycles, and the Pid option on the sandbox
// is node or container level, the infra container is not needed.
func (s *Sandbox) NeedsInfra(serverDropsInfra bool) bool {
	return !serverDropsInfra || s.nsOpts.Pid == types.NamespaceMode_POD
}
