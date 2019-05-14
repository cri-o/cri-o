package oci

import (
	"bytes"
	"fmt"
	"io"
	"syscall"
	"time"

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

	// BufSize is the size of buffers passed in to sockets
	BufSize = 8192

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
	defaultRuntime           RuntimeHandler
	runtimes                 map[string]RuntimeHandler
	conmonPath               string
	conmonEnv                []string
	conmonCgroup             string
	cgroupManager            string
	containerExitsDir        string
	containerAttachSocketDir string
	logSizeMax               int64
	logToJournald            bool
	noPivot                  bool
	ctrStopTimeout           int64

	runtimeImplList map[string]RuntimeImpl
}

// RuntimeImpl is an interface used by the caller to interact with the
// container runtime. The purpose of this interface being to abstract
// implementations and their associated assumptions regarding the way to
// interact with containers. This will allow for new implementations of
// this interface, especially useful for the case of VM based container
// runtimes. Assumptions based on the fact that a container process runs
// on the host will be limited to the RuntimeOCI implementation.
type RuntimeImpl interface {
	CreateContainer(*Container, string) error
	StartContainer(*Container) error
	ExecContainer(*Container, []string, io.Reader, io.WriteCloser, io.WriteCloser,
		bool, <-chan remotecommand.TerminalSize) error
	ExecSyncContainer(*Container, []string, int64) (*ExecSyncResponse, error)
	UpdateContainer(*Container, *rspec.LinuxResources) error
	StopContainer(context.Context, *Container, int64) error
	DeleteContainer(*Container) error
	UpdateContainerStatus(*Container) error
	PauseContainer(*Container) error
	UnpauseContainer(*Container) error
	ContainerStats(*Container) (*ContainerStats, error)
	SignalContainer(*Container, syscall.Signal) error
	AttachContainer(*Container, io.Reader, io.WriteCloser, io.WriteCloser,
		bool, <-chan remotecommand.TerminalSize) error
	PortForwardContainer(*Container, int32, io.ReadWriter) error
	ReopenContainerLog(*Container) error
	WaitContainerStateStopped(context.Context, *Container) error
}

// RuntimeHandler represents each item of the "crio.runtime.runtimes" TOML
// config table.
type RuntimeHandler struct {
	RuntimePath string `toml:"runtime_path"`
	RuntimeType string `toml:"runtime_type"`
	RuntimeRoot string `toml:"runtime_root"`
}

// New creates a new Runtime with options provided
func New(defaultRuntime string,
	runtimes map[string]RuntimeHandler,
	conmonPath string,
	conmonEnv []string,
	conmonCgroup,
	cgroupManager,
	containerExitsDir,
	containerAttachSocketDir string,
	logSizeMax int64,
	logToJournald,
	noPivot bool,
	ctrStopTimeout int64) (*Runtime, error) {

	defRuntime, ok := runtimes[defaultRuntime]
	if !ok {
		return nil, fmt.Errorf("no runtime configured for default_runtime=%q", defaultRuntime)
	}
	if defRuntime.RuntimePath == "" {
		return nil, fmt.Errorf("empty runtime path for default_runtime=%q", defaultRuntime)
	}

	return &Runtime{
		defaultRuntime:           defRuntime,
		runtimes:                 runtimes,
		conmonPath:               conmonPath,
		conmonEnv:                conmonEnv,
		conmonCgroup:             conmonCgroup,
		cgroupManager:            cgroupManager,
		containerExitsDir:        containerExitsDir,
		containerAttachSocketDir: containerAttachSocketDir,
		logSizeMax:               logSizeMax,
		logToJournald:            logToJournald,
		noPivot:                  noPivot,
		ctrStopTimeout:           ctrStopTimeout,
		runtimeImplList:          make(map[string]RuntimeImpl),
	}, nil
}

// Runtimes returns the map of OCI runtimes.
func (r *Runtime) Runtimes() map[string]RuntimeHandler {
	return r.runtimes
}

// ValidateRuntimeHandler returns an error if the runtime handler string
// provided does not match any valid use case.
func (r *Runtime) ValidateRuntimeHandler(handler string) (RuntimeHandler, error) {
	if handler == "" {
		return RuntimeHandler{}, fmt.Errorf("empty runtime handler")
	}

	runtimeHandler, ok := r.runtimes[handler]
	if !ok {
		return RuntimeHandler{}, fmt.Errorf("failed to find runtime handler %s from runtime list %v",
			handler, r.runtimes)
	}
	if runtimeHandler.RuntimePath == "" {
		return RuntimeHandler{}, fmt.Errorf("empty runtime path for runtime handler %s", handler)
	}

	return runtimeHandler, nil
}

// WaitContainerStateStopped runs a loop polling UpdateStatus(), seeking for
// the container status to be updated to 'stopped'. Either it gets the expected
// status and returns nil, or it reaches the timeout and returns an error.
func (r *Runtime) WaitContainerStateStopped(ctx context.Context, c *Container) (err error) {
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
	timeout := r.ctrStopTimeout
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

func (r *Runtime) newRuntimeImpl(c *Container) (RuntimeImpl, error) {
	// Define the current runtime handler as the default runtime handler.
	rh := r.defaultRuntime

	// Override the current runtime handler with the runtime handler
	// corresponding to the runtime handler key provided with this
	// specific container.
	if c.runtimeHandler != "" {
		runtimeHandler, err := r.ValidateRuntimeHandler(c.runtimeHandler)
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
	return newRuntimeOCI(r, &rh), nil
}

// RuntimeImpl returns the runtime implementation for a given container
func (r *Runtime) RuntimeImpl(c *Container) (RuntimeImpl, error) {
	impl, ok := r.runtimeImplList[c.ID()]
	if !ok {
		return r.newRuntimeImpl(c)
	}

	return impl, nil
}

// CreateContainer creates a container.
func (r *Runtime) CreateContainer(c *Container, cgroupParent string) error {
	// Instantiate a new runtime implementation for this new container
	impl, err := r.newRuntimeImpl(c)
	if err != nil {
		return err
	}

	// Assign this runtime implementation to the current container
	r.runtimeImplList[c.ID()] = impl

	return impl.CreateContainer(c, cgroupParent)
}

// StartContainer starts a container.
func (r *Runtime) StartContainer(c *Container) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	return impl.StartContainer(c)
}

// ExecContainer prepares a streaming endpoint to execute a command in the container.
func (r *Runtime) ExecContainer(c *Container, cmd []string, stdin io.Reader, stdout, stderr io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	return impl.ExecContainer(c, cmd, stdin, stdout, stderr, tty, resize)
}

// ExecSyncContainer execs a command in a container and returns it's stdout, stderr and return code.
func (r *Runtime) ExecSyncContainer(c *Container, command []string, timeout int64) (*ExecSyncResponse, error) {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return nil, err
	}

	return impl.ExecSyncContainer(c, command, timeout)
}

// UpdateContainer updates container resources
func (r *Runtime) UpdateContainer(c *Container, res *rspec.LinuxResources) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	return impl.UpdateContainer(c, res)
}

// StopContainer stops a container. Timeout is given in seconds.
func (r *Runtime) StopContainer(ctx context.Context, c *Container, timeout int64) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	return impl.StopContainer(ctx, c, timeout)
}

// DeleteContainer deletes a container.
func (r *Runtime) DeleteContainer(c *Container) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	defer delete(r.runtimeImplList, c.ID())

	return impl.DeleteContainer(c)
}

// UpdateContainerStatus refreshes the status of the container.
func (r *Runtime) UpdateContainerStatus(c *Container) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	return impl.UpdateContainerStatus(c)
}

// PauseContainer pauses a container.
func (r *Runtime) PauseContainer(c *Container) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	return impl.PauseContainer(c)
}

// UnpauseContainer unpauses a container.
func (r *Runtime) UnpauseContainer(c *Container) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	return impl.UnpauseContainer(c)
}

// ContainerStats provides statistics of a container.
func (r *Runtime) ContainerStats(c *Container) (*ContainerStats, error) {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return nil, err
	}

	return impl.ContainerStats(c)
}

// SignalContainer sends a signal to a container process.
func (r *Runtime) SignalContainer(c *Container, sig syscall.Signal) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	return impl.SignalContainer(c, sig)
}

// AttachContainer attaches IO to a running container.
func (r *Runtime) AttachContainer(c *Container, inputStream io.Reader, outputStream, errorStream io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	return impl.AttachContainer(c, inputStream, outputStream, errorStream, tty, resize)
}

// PortForwardContainer forwards the specified port provides statistics of a container.
func (r *Runtime) PortForwardContainer(c *Container, port int32, stream io.ReadWriter) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	return impl.PortForwardContainer(c, port, stream)
}

// ReopenContainerLog reopens the log file of a container.
func (r *Runtime) ReopenContainerLog(c *Container) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	return impl.ReopenContainerLog(c)
}

// ExecSyncResponse is returned from ExecSync.
type ExecSyncResponse struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int32
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
