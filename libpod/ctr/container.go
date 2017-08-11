package ctr

import (
	"fmt"
)

var (
	// ErrNotImplemented indicates that functionality is not yet implemented
	ErrNotImplemented = fmt.Errorf("NOT IMPLEMENTED")
)

// Container is a single OCI container
type Container struct {
	// TODO populate
}

// Create creates a container in the OCI runtime
func (c *Container) Create() error {
	return ErrNotImplemented
}

// Start starts a container
func (c *Container) Start() error {
	return ErrNotImplemented
}

// Stop stops a container
func (c *Container) Stop() error {
	return ErrNotImplemented
}

// Kill sends a signal to a container
func (c *Container) Kill(signal uint) error {
	return ErrNotImplemented
}

// Exec starts a new process inside the container
// TODO does this need arguments?
// TODO should this return anything besides error?
func (c *Container) Exec() error {
	return ErrNotImplemented
}

// Attach attaches to a container
// TODO does this need arguments?
// TODO should this return anything besides error?
func (c *Container) Attach() error {
	return ErrNotImplemented
}

// Mount mounts a container's filesystem on the host
// The path where the container has been mounted is returned
func (c *Container) Mount() (string, error) {
	return "", ErrNotImplemented
}

// Status gets a container's status
// TODO this should return relevant information about container state
func (c *Container) Status() error {
	return ErrNotImplemented
}
