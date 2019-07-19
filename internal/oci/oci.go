package oci

import (
	"bytes"
	"fmt"
	"io"
	"sync"
	"syscall"
	"time"

	"github.com/cri-o/cri-o/internal/lib/config"
	"github.com/cri-o/cri-o/pkg/oci"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/net/context"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	// ContainerStateCreated represents the created state of a container
	ContainerStateCreated = "created"
	// ContainerStatePaused represents the paused state of a container
	ContainerStatePaused = "paused"
	// ContainerStateRunning represents the running state of a container
	ContainerStateRunning = "running"
	// ContainerStateStopped represents the stopped state of a container
	ContainerStateStopped = "stopped"
	// ContainerCreateTimeout represents the value of container creating timeout
	ContainerCreateTimeout = 240 * time.Second

	// CgroupfsCgroupsManager represents cgroupfs native cgroup manager
	CgroupfsCgroupsManager = "cgroupfs"
	// SystemdCgroupsManager represents systemd native cgroup manager
	SystemdCgroupsManager = "systemd"

	// killContainerTimeout is the timeout that we wait for the container to
	// be SIGKILLed.
	killContainerTimeout = 2 * time.Minute

	// minCtrStopTimeout is the minimal amount of time in seconds to wait
	// before issuing a timeout regarding the proper termination of the
	// container.
	minCtrStopTimeout = 30
)

// Runtime is the generic structure holding both global and specific
// information about the runtime.
type Runtime struct {
	config              *config.Config
	runtimeImplMap      map[string]oci.RuntimeImpl
	runtimeImplMapMutex sync.RWMutex
}

// New creates a new Runtime with options provided
func New(c *config.Config) *Runtime {
	return &Runtime{
		config:         c,
		runtimeImplMap: make(map[string]oci.RuntimeImpl),
	}
}

// Runtimes returns the map of OCI runtimes.
func (r *Runtime) Runtimes() config.Runtimes {
	return r.config.Runtimes
}

// ValidateRuntimeHandler returns an error if the runtime handler string
// provided does not match any valid use case.
func (r *Runtime) ValidateRuntimeHandler(handler string) (*config.RuntimeHandler, error) {
	if handler == "" {
		return nil, fmt.Errorf("empty runtime handler")
	}

	runtimeHandler, ok := r.config.Runtimes[handler]
	if !ok {
		return nil, fmt.Errorf("failed to find runtime handler %s from runtime list %v",
			handler, r.config.Runtimes)
	}
	if runtimeHandler.RuntimePath == "" {
		return nil, fmt.Errorf("empty runtime path for runtime handler %s", handler)
	}

	return runtimeHandler, nil
}

// WaitContainerStateStopped runs a loop polling UpdateStatus(), seeking for
// the container status to be updated to 'stopped'. Either it gets the expected
// status and returns nil, or it reaches the timeout and returns an error.
func (r *Runtime) WaitContainerStateStopped(ctx context.Context, c *oci.Container) (err error) {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	// No need to go further and spawn the go routine if the container
	// is already in the expected status.
	if c.State().Status == ContainerStateStopped {
		return nil
	}

	// We need to ensure the container termination will be properly waited
	// for by defining a minimal timeout value. This will prevent timeout
	// value defined in the configuration file to be too low.
	timeout := r.config.CtrStopTimeout
	if timeout < minCtrStopTimeout {
		timeout = minCtrStopTimeout
	}

	done := make(chan error)
	chControl := make(chan struct{})
	go func() {
		for {
			select {
			case <-chControl:
				return
			default:
				// Check if the container is stopped
				if err := impl.UpdateContainerStatus(c); err != nil {
					done <- err
					close(done)
					return
				}
				if c.State().Status == ContainerStateStopped {
					close(done)
					return
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()
	select {
	case err = <-done:
		break
	case <-ctx.Done():
		close(chControl)
		return ctx.Err()
	case <-time.After(time.Duration(timeout) * time.Second):
		close(chControl)
		return fmt.Errorf("failed to get container stopped status: %ds timeout reached", timeout)
	}

	if err != nil {
		return fmt.Errorf("failed to get container stopped status: %v", err)
	}

	return nil
}

func (r *Runtime) newRuntimeImpl(c *oci.Container) (oci.RuntimeImpl, error) {
	// Define the current runtime handler as the default runtime handler.
	rh := r.config.Runtimes[r.config.DefaultRuntime]

	// Override the current runtime handler with the runtime handler
	// corresponding to the runtime handler key provided with this
	// specific container.
	if c.RuntimeHandler() != "" {
		runtimeHandler, err := r.ValidateRuntimeHandler(c.RuntimeHandler())
		if err != nil {
			return nil, err
		}

		rh = runtimeHandler
	}

	if rh.RuntimeType == RuntimeTypeVM {
		return newRuntimeVM(rh.RuntimePath), nil
	}

	// If the runtime type is different from "vm", then let's fallback
	// onto the OCI implementation by default.
	return newRuntimeOCI(r, rh), nil
}

// RuntimeImpl returns the runtime implementation for a given container
func (r *Runtime) RuntimeImpl(c *oci.Container) (oci.RuntimeImpl, error) {
	r.runtimeImplMapMutex.RLock()
	impl, ok := r.runtimeImplMap[c.ID()]
	r.runtimeImplMapMutex.RUnlock()
	if !ok {
		return r.newRuntimeImpl(c)
	}

	return impl, nil
}

// CreateContainer creates a container.
func (r *Runtime) CreateContainer(c *oci.Container, cgroupParent string) error {
	// Instantiate a new runtime implementation for this new container
	impl, err := r.newRuntimeImpl(c)
	if err != nil {
		return err
	}

	// Assign this runtime implementation to the current container
	r.runtimeImplMapMutex.Lock()
	r.runtimeImplMap[c.ID()] = impl
	r.runtimeImplMapMutex.Unlock()

	return impl.CreateContainer(c, cgroupParent)
}

// StartContainer starts a container.
func (r *Runtime) StartContainer(c *oci.Container) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	return impl.StartContainer(c)
}

// ExecContainer prepares a streaming endpoint to execute a command in the container.
func (r *Runtime) ExecContainer(c *oci.Container, cmd []string, stdin io.Reader, stdout, stderr io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	return impl.ExecContainer(c, cmd, stdin, stdout, stderr, tty, resize)
}

// ExecSyncContainer execs a command in a container and returns it's stdout, stderr and return code.
func (r *Runtime) ExecSyncContainer(c *oci.Container, command []string, timeout int64) (*oci.ExecSyncResponse, error) {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return nil, err
	}

	return impl.ExecSyncContainer(c, command, timeout)
}

// UpdateContainer updates container resources
func (r *Runtime) UpdateContainer(c *oci.Container, res *rspec.LinuxResources) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	return impl.UpdateContainer(c, res)
}

// StopContainer stops a container. Timeout is given in seconds.
func (r *Runtime) StopContainer(ctx context.Context, c *oci.Container, timeout int64) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	return impl.StopContainer(ctx, c, timeout)
}

// DeleteContainer deletes a container.
func (r *Runtime) DeleteContainer(c *oci.Container) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	defer func() {
		r.runtimeImplMapMutex.Lock()
		delete(r.runtimeImplMap, c.ID())
		r.runtimeImplMapMutex.Unlock()
	}()

	return impl.DeleteContainer(c)
}

// UpdateContainerStatus refreshes the status of the container.
func (r *Runtime) UpdateContainerStatus(c *oci.Container) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	return impl.UpdateContainerStatus(c)
}

// PauseContainer pauses a container.
func (r *Runtime) PauseContainer(c *oci.Container) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	return impl.PauseContainer(c)
}

// UnpauseContainer unpauses a container.
func (r *Runtime) UnpauseContainer(c *oci.Container) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	return impl.UnpauseContainer(c)
}

// ContainerStats provides statistics of a container.
func (r *Runtime) ContainerStats(c *oci.Container) (*oci.ContainerStats, error) {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return nil, err
	}

	return impl.ContainerStats(c)
}

// SignalContainer sends a signal to a container process.
func (r *Runtime) SignalContainer(c *oci.Container, sig syscall.Signal) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	return impl.SignalContainer(c, sig)
}

// AttachContainer attaches IO to a running container.
func (r *Runtime) AttachContainer(c *oci.Container, inputStream io.Reader, outputStream, errorStream io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	return impl.AttachContainer(c, inputStream, outputStream, errorStream, tty, resize)
}

// PortForwardContainer forwards the specified port provides statistics of a container.
func (r *Runtime) PortForwardContainer(c *oci.Container, port int32, stream io.ReadWriter) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	return impl.PortForwardContainer(c, port, stream)
}

// ReopenContainerLog reopens the log file of a container.
func (r *Runtime) ReopenContainerLog(c *oci.Container) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	return impl.ReopenContainerLog(c)
}

// ExecSyncError wraps command's streams, exit code and error on ExecSync error.
type ExecSyncError struct {
	Stdout   bytes.Buffer
	Stderr   bytes.Buffer
	ExitCode int32
	Err      error
}

func (e *ExecSyncError) Error() string {
	return fmt.Sprintf("command error: %+v, stdout: %s, stderr: %s, exit code %d", e.Err, e.Stdout.Bytes(), e.Stderr.Bytes(), e.ExitCode)
}
