package oci

import (
	"context"
	"errors"
	"fmt"
)

// ErrContainerNotRunning is returned when the runtime reports the container is not running.
var ErrContainerNotRunning = errors.New("container is not running")

// ErrContainerStateUnknown wraps the cause when the runtime cannot determine the state.
type ErrContainerStateUnknown struct {
	Err error
}

// Error implements the error interface.
func (e *ErrContainerStateUnknown) Error() string {
	return fmt.Sprintf("unable to determine whether the container is running: %v", e.Err)
}

// Unwrap returns the underlying error.
func (e *ErrContainerStateUnknown) Unwrap() error {
	return e.Err
}

// checkContainerAlive asks the runtime whether the container can take a new
// process. A runtime which cannot tell reports ErrContainerStateUnknown, a
// container which is gone reports ErrContainerNotRunning.
func checkContainerAlive(ctx context.Context, rt RuntimeImpl, c *Container) error {
	alive, err := rt.IsContainerAlive(ctx, c)
	if err != nil {
		return &ErrContainerStateUnknown{Err: err}
	}

	if !alive {
		return fmt.Errorf("%w: %s", ErrContainerNotRunning, c.ID())
	}

	return nil
}
