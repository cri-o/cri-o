package oci

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cri-o/cri-o/pkg/config"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

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

	// killContainerTimeout is the timeout that we wait for the container to
	// be SIGKILLed.
	killContainerTimeout = 2 * time.Minute
)

// Runtime is the generic structure holding both global and specific
// information about the runtime.
type Runtime struct {
	config              *config.Config
	runtimeImplMap      map[string]RuntimeImpl
	runtimeImplMapMutex sync.RWMutex
}

// RuntimeImpl is an interface used by the caller to interact with the
// container runtime. The purpose of this interface being to abstract
// implementations and their associated assumptions regarding the way to
// interact with containers. This will allow for new implementations of
// this interface, especially useful for the case of VM based container
// runtimes. Assumptions based on the fact that a container process runs
// on the host will be limited to the RuntimeOCI implementation.
type RuntimeImpl interface {
	CreateContainer(context.Context, *Container, string) error
	StartContainer(context.Context, *Container) error
	ExecContainer(context.Context, *Container, []string, io.Reader, io.WriteCloser, io.WriteCloser,
		bool, <-chan remotecommand.TerminalSize) error
	ExecSyncContainer(context.Context, *Container, []string, int64) (*types.ExecSyncResponse, error)
	UpdateContainer(context.Context, *Container, *rspec.LinuxResources) error
	StopContainer(context.Context, *Container, int64) error
	DeleteContainer(context.Context, *Container) error
	UpdateContainerStatus(context.Context, *Container) error
	PauseContainer(context.Context, *Container) error
	UnpauseContainer(context.Context, *Container) error
	ContainerStats(context.Context, *Container, string) (*types.ContainerStats, error)
	SignalContainer(context.Context, *Container, syscall.Signal) error
	AttachContainer(context.Context, *Container, io.Reader, io.WriteCloser, io.WriteCloser,
		bool, <-chan remotecommand.TerminalSize) error
	PortForwardContainer(context.Context, *Container, string,
		int32, io.ReadWriteCloser) error
	ReopenContainerLog(context.Context, *Container) error
	Shutdown() error
}

// New creates a new Runtime with options provided
func New(c *config.Config) (*Runtime, error) {
	execNotifyDir := filepath.Join(c.ContainerAttachSocketDir, "exec-pid-dir")
	if err := os.MkdirAll(execNotifyDir, 0o750); err != nil {
		return nil, errors.Wrapf(err, "create oci runtime pid dir")
	}

	return &Runtime{
		config:         c,
		runtimeImplMap: make(map[string]RuntimeImpl),
	}, nil
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

func (r *Runtime) getRuntimeHandler(handler string) (*config.RuntimeHandler, error) {
	// Define the current runtime handler as the default runtime handler.
	rh := r.config.Runtimes[r.config.DefaultRuntime]

	// Override the current runtime handler with the runtime handler
	// corresponding to the runtime handler key provided with this
	// specific container.
	if handler != "" {
		runtimeHandler, err := r.ValidateRuntimeHandler(handler)
		if err != nil {
			return nil, err
		}

		rh = runtimeHandler
	}

	return rh, nil
}

// PrivelegedRuntimeHandler returns a boolean value configured for the
// runtimeHandler indicating if devices on the host are passed/not passed
// to a container running as privileged.
func (r *Runtime) PrivilegedWithoutHostDevices(handler string) (bool, error) {
	rh, err := r.getRuntimeHandler(handler)
	if err != nil {
		return false, err
	}

	return rh.PrivilegedWithoutHostDevices, nil
}

// AllowedAnnotations returns the allowed annotations for this runtime.
func (r *Runtime) AllowedAnnotations(handler string) ([]string, error) {
	rh, err := r.getRuntimeHandler(handler)
	if err != nil {
		return []string{}, err
	}

	return rh.AllowedAnnotations, nil
}

// RuntimeType returns the type of runtimeHandler
// This is needed when callers need to do specific work for oci vs vm
// containers, like monitor an oci container's conmon.
func (r *Runtime) RuntimeType(runtimeHandler string) (string, error) {
	rh, err := r.getRuntimeHandler(runtimeHandler)
	if err != nil {
		return "", err
	}

	return rh.RuntimeType, nil
}

func (r *Runtime) newRuntimeImpl(c *Container) (RuntimeImpl, error) {
	rh, err := r.getRuntimeHandler(c.runtimeHandler)
	if err != nil {
		return nil, err
	}

	if rh.RuntimeType == config.RuntimeTypeVM {
		return newRuntimeVM(rh.RuntimePath, rh.RuntimeRoot, rh.RuntimeConfigPath), nil
	}

	// If the runtime type is different from "vm", then let's fallback
	// onto the OCI implementation by default.
	return newRuntimeOCI(r, rh), nil
}

// RuntimeImpl returns the runtime implementation for a given container
func (r *Runtime) RuntimeImpl(c *Container) (RuntimeImpl, error) {
	r.runtimeImplMapMutex.RLock()
	impl, ok := r.runtimeImplMap[c.Sandbox()]
	r.runtimeImplMapMutex.RUnlock()
	if ok {
		return impl, nil
	}

	impl, err := r.newRuntimeImpl(c)
	if err != nil {
		return nil, err
	}
	r.runtimeImplMapMutex.Lock()
	r.runtimeImplMap[c.Sandbox()] = impl
	r.runtimeImplMapMutex.Unlock()
	return impl, nil
}

func (r *Runtime) RemoveRuntimeForSandbox(sandboxID string) error {
	r.runtimeImplMapMutex.Lock()
	defer r.runtimeImplMapMutex.Unlock()
	impl, ok := r.runtimeImplMap[sandboxID]
	if !ok {
		return nil
	}
	if err := impl.Shutdown(); err != nil {
		return err
	}
	delete(r.runtimeImplMap, sandboxID)
	return nil
}

// CreateContainer creates a container.
func (r *Runtime) CreateContainer(ctx context.Context, c *Container, cgroupParent string) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	return impl.CreateContainer(ctx, c, cgroupParent)
}

// StartContainer starts a container.
func (r *Runtime) StartContainer(ctx context.Context, c *Container) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	return impl.StartContainer(ctx, c)
}

// ExecContainer prepares a streaming endpoint to execute a command in the container.
func (r *Runtime) ExecContainer(ctx context.Context, c *Container, cmd []string, stdin io.Reader, stdout, stderr io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	return impl.ExecContainer(ctx, c, cmd, stdin, stdout, stderr, tty, resize)
}

// ExecSyncContainer execs a command in a container and returns it's stdout, stderr and return code.
func (r *Runtime) ExecSyncContainer(ctx context.Context, c *Container, command []string, timeout int64) (*types.ExecSyncResponse, error) {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return nil, err
	}

	return impl.ExecSyncContainer(ctx, c, command, timeout)
}

// UpdateContainer updates container resources
func (r *Runtime) UpdateContainer(ctx context.Context, c *Container, res *rspec.LinuxResources) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	return impl.UpdateContainer(ctx, c, res)
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
func (r *Runtime) DeleteContainer(ctx context.Context, c *Container) (err error) {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	return impl.DeleteContainer(ctx, c)
}

// UpdateContainerStatus refreshes the status of the container.
func (r *Runtime) UpdateContainerStatus(ctx context.Context, c *Container) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	return impl.UpdateContainerStatus(ctx, c)
}

// PauseContainer pauses a container.
func (r *Runtime) PauseContainer(ctx context.Context, c *Container) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	return impl.PauseContainer(ctx, c)
}

// UnpauseContainer unpauses a container.
func (r *Runtime) UnpauseContainer(ctx context.Context, c *Container) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	return impl.UnpauseContainer(ctx, c)
}

// ContainerStats provides statistics of a container.
func (r *Runtime) ContainerStats(ctx context.Context, c *Container, cgroup string) (*types.ContainerStats, error) {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return nil, err
	}

	return impl.ContainerStats(ctx, c, cgroup)
}

// SignalContainer sends a signal to a container process.
func (r *Runtime) SignalContainer(ctx context.Context, c *Container, sig syscall.Signal) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	return impl.SignalContainer(ctx, c, sig)
}

// AttachContainer attaches IO to a running container.
func (r *Runtime) AttachContainer(ctx context.Context, c *Container, inputStream io.Reader, outputStream, errorStream io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	return impl.AttachContainer(ctx, c, inputStream, outputStream, errorStream, tty, resize)
}

// PortForwardContainer forwards the specified port provides statistics of a container.
func (r *Runtime) PortForwardContainer(ctx context.Context, c *Container, netNsPath string, port int32, stream io.ReadWriteCloser) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	return impl.PortForwardContainer(ctx, c, netNsPath, port, stream)
}

// ReopenContainerLog reopens the log file of a container.
func (r *Runtime) ReopenContainerLog(ctx context.Context, c *Container) error {
	impl, err := r.RuntimeImpl(c)
	if err != nil {
		return err
	}

	return impl.ReopenContainerLog(ctx, c)
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

// BuildContainerdBinaryName() is responsible for ensuring the binary passed will
// be properly converted to the containerd binary naming pattern.
//
// This method should never ever be called from anywhere else but the runtimeVM,
// and the only reason its exported here is in order to get some test coverage.
func BuildContainerdBinaryName(path string) string {
	// containerd expects the runtime name to be in the following pattern:
	//        ($dir.)?$prefix.$name.$version
	//        -------- ------ ----- ---------
	//              |     |     |      |
	//              v     |     |      |
	//      "/usr/local/bin"    |      |
	//        (optional)  |     |      |
	//                    v     |      |
	//             "containerd.shim."  |
	//                          |      |
	//                          v      |
	//                     "kata-qemu" |
	//                                 v
	//                                "v2"
	const expectedPrefix = "containerd-shim-"
	const expectedVersion = "-v2"

	const binaryPrefix = "containerd.shim"
	const binaryVersion = "v2"

	runtimeDir := filepath.Dir(path)
	// This is only safe to do because the runtime_path, for the VM runtime_type, is validated in the config,
	// allowing us to take the liberty to simply go ahead and check, without having to ensure we're receiving
	// the binary in the expected form.
	//
	// For clarity, it could be ensured twice, but we count on the developer to never ever call this function
	// in a different context from the one used in the runtime_vm.go file.
	runtimeName := strings.SplitAfter(strings.Split(filepath.Base(path), expectedVersion)[0], expectedPrefix)[1]

	return filepath.Join(runtimeDir, fmt.Sprintf("%s.%s.%s", binaryPrefix, runtimeName, binaryVersion))
}
