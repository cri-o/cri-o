//go:build !linux && !freebsd

package sandbox

import (
	"context"
)

// UnmountShm removes the shared memory mount for the sandbox and returns an
// error if any failure occurs.
func (s *Sandbox) UnmountShm(ctx context.Context) error {
	return nil
}

// NeedsInfra is a function that returns whether the sandbox will need an infra container.
// If the server manages the namespace lifecycles, and the Pid option on the sandbox
// is node or container level, the infra container is not needed
func (s *Sandbox) NeedsInfra(serverDropsInfra bool) bool {
	return false
}
