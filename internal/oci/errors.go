package oci

import (
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
