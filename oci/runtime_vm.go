package oci

import (
	"io"
	"syscall"

	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/net/context"
	"k8s.io/client-go/tools/remotecommand"
)

// RuntimeVM is the Runtime interface implementation that is more appropriate
// for VM based container runtimes.
type RuntimeVM struct {
	RuntimeBase
}

// NewRuntimeVM creates a new RuntimeVM instance
func NewRuntimeVM(rb RuntimeBase) (RuntimeImpl, error) {
	return &RuntimeVM{
		RuntimeBase: rb,
	}, nil
}

// CreateContainer creates a container.
func (r *RuntimeVM) CreateContainer(c *Container, cgroupParent string) (err error) {
	return nil
}

// StartContainer starts a container.
func (r *RuntimeVM) StartContainer(c *Container) error {
	return nil
}

// ExecContainer prepares a streaming endpoint to execute a command in the container.
func (r *RuntimeVM) ExecContainer(c *Container, cmd []string, stdin io.Reader, stdout, stderr io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) error {
	return nil
}

// ExecSyncContainer execs a command in a container and returns it's stdout, stderr and return code.
func (r *RuntimeVM) ExecSyncContainer(c *Container, command []string, timeout int64) (resp *ExecSyncResponse, err error) {
	return &ExecSyncResponse{}, nil
}

// UpdateContainer updates container resources
func (r *RuntimeVM) UpdateContainer(c *Container, res *rspec.LinuxResources) error {
	return nil
}

// WaitContainerStateStopped runs a loop polling UpdateStatus(), seeking for
// the container status to be updated to 'stopped'. Either it gets the expected
// status and returns nil, or it reaches the timeout and returns an error.
func (r *RuntimeVM) WaitContainerStateStopped(ctx context.Context, c *Container) (err error) {
	return nil
}

// StopContainer stops a container. Timeout is given in seconds.
func (r *RuntimeVM) StopContainer(ctx context.Context, c *Container, timeout int64) error {
	return nil
}

// DeleteContainer deletes a container.
func (r *RuntimeVM) DeleteContainer(c *Container) error {
	return nil
}

// UpdateContainerStatus refreshes the status of the container.
func (r *RuntimeVM) UpdateContainerStatus(c *Container) error {
	return nil
}

// PauseContainer pauses a container.
func (r *RuntimeVM) PauseContainer(c *Container) error {
	return nil
}

// UnpauseContainer unpauses a container.
func (r *RuntimeVM) UnpauseContainer(c *Container) error {
	return nil
}

// ContainerStats provides statistics of a container.
func (r *RuntimeVM) ContainerStats(c *Container) (*ContainerStats, error) {
	return &ContainerStats{}, nil
}

// SignalContainer sends a signal to a container process.
func (r *RuntimeVM) SignalContainer(c *Container, sig syscall.Signal) error {
	return nil
}

// AttachContainer attaches IO to a running container.
func (r *RuntimeVM) AttachContainer(c *Container, inputStream io.Reader, outputStream, errorStream io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) error {
	return nil
}

// PortForwardContainer forwards the specified port provides statistics of a container.
func (r *RuntimeVM) PortForwardContainer(c *Container, port int32, stream io.ReadWriter) error {
	return nil
}

// ReopenContainerLog reopens the log file of a container.
func (r *RuntimeVM) ReopenContainerLog(c *Container) error {
	return nil
}
