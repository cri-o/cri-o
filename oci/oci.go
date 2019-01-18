package oci

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
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

	// UntrustedRuntime is the implicit runtime handler name used to
	// fallback to the untrusted runtime.
	UntrustedRuntime = "untrusted"
)

// Runtime is the generic structure holding both global and specific
// information about the runtime through RuntimeBase and RuntimeImpl
// respectively.
type Runtime struct {
	RuntimeBase
	RuntimeImpl
}

// RuntimeBase gathers information that can be shared across Runtime
// implementations.
type RuntimeBase struct {
	name                     string
	trustedPath              string
	untrustedPath            string
	trustLevel               string
	runtimes                 map[string]RuntimeHandler
	conmonPath               string
	conmonEnv                []string
	cgroupManager            string
	containerExitsDir        string
	containerAttachSocketDir string
	logSizeMax               int64
	noPivot                  bool
	ctrStopTimeout           int64
}

// RuntimeImpl is an interface used by the caller to interact with the
// container runtime. The purpose of this interface being to abstract
// implementations and their associated assumptions regarding the way to
// interact with containers. This will allow for new implementations of
// this interface, especially useful for the case of VM based container
// runtimes. Assumptions based on the fact that a container process runs
// on the host will be limited to the RuntimeV1 implementation.
type RuntimeImpl interface {
	Version() (string, error)
	CreateContainer(*Container, string) error
	StartContainer(*Container) error
	ExecContainer(*Container, []string, io.Reader, io.WriteCloser, io.WriteCloser,
		bool, <-chan remotecommand.TerminalSize) error
	ExecSyncContainer(*Container, []string, int64) (*ExecSyncResponse, error)
	UpdateContainer(*Container, *rspec.LinuxResources) error
	WaitContainerStateStopped(context.Context, *Container) error
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
}

// RuntimeHandler represents each item of the "crio.runtime.runtimes" TOML
// config table.
type RuntimeHandler struct {
	RuntimePath string `toml:"runtime_path"`
}

// New creates a new Runtime with options provided
func New(runtimeTrustedPath string,
	runtimeUntrustedPath string,
	trustLevel string,
	defaultRuntime string,
	runtimes map[string]RuntimeHandler,
	conmonPath string,
	conmonEnv []string,
	cgroupManager string,
	containerExitsDir string,
	containerAttachSocketDir string,
	logSizeMax int64,
	noPivot bool,
	ctrStopTimeout int64,
	runtimeVersion string) (*Runtime, error) {
	if runtimeTrustedPath == "" {
		// this means no "runtime" key in config as it's deprecated, fallback to
		// the runtime handler configured as default.
		r, ok := runtimes[defaultRuntime]
		if !ok {
			return nil, fmt.Errorf("no runtime configured for default_runtime=%q", defaultRuntime)
		}
		runtimeTrustedPath = r.RuntimePath
	}

	rb := RuntimeBase{
		name:                     filepath.Base(runtimeTrustedPath),
		trustedPath:              runtimeTrustedPath,
		untrustedPath:            runtimeUntrustedPath,
		trustLevel:               trustLevel,
		runtimes:                 runtimes,
		conmonPath:               conmonPath,
		conmonEnv:                conmonEnv,
		cgroupManager:            cgroupManager,
		containerExitsDir:        containerExitsDir,
		containerAttachSocketDir: containerAttachSocketDir,
		logSizeMax:               logSizeMax,
		noPivot:                  noPivot,
		ctrStopTimeout:           ctrStopTimeout,
	}

	ri, err := newRuntimeImpl(runtimeVersion, rb)
	if err != nil {
		return nil, err
	}

	return &Runtime{
		RuntimeBase: rb,
		RuntimeImpl: ri,
	}, nil
}

// newRuntimeImpl creates a new Runtime implementation based on the version.
func newRuntimeImpl(runtimeVersion string, rb RuntimeBase) (RuntimeImpl, error) {
	return NewRuntimeV1(rb)
}

// Name returns the name of the OCI Runtime
func (r *RuntimeBase) Name() string {
	return r.name
}

// Runtimes returns the map of OCI runtimes.
func (r *RuntimeBase) Runtimes() map[string]RuntimeHandler {
	return r.runtimes
}

// ValidateRuntimeHandler returns an error if the runtime handler string
// provided does not match any valid use case.
func (r *RuntimeBase) ValidateRuntimeHandler(handler string) (RuntimeHandler, error) {
	if handler == "" {
		return RuntimeHandler{}, fmt.Errorf("empty runtime handler")
	}

	runtimeHandler, ok := r.runtimes[handler]
	if !ok {
		if handler == UntrustedRuntime && r.untrustedPath != "" {
			return RuntimeHandler{
				RuntimePath: r.untrustedPath,
			}, nil
		}
		return RuntimeHandler{}, fmt.Errorf("failed to find runtime handler %s from runtime list %v",
			handler, r.runtimes)
	}
	if runtimeHandler.RuntimePath == "" {
		return RuntimeHandler{}, fmt.Errorf("empty runtime path for runtime handler %s", handler)
	}

	return runtimeHandler, nil
}

// path returns the full path the OCI Runtime executable.
// Depending if the container is privileged and/or trusted,
// this will return either the trusted or untrusted runtime path.
func (r *RuntimeBase) path(c *Container) (string, error) {
	if c.runtimeHandler != "" {
		runtimeHandler, err := r.ValidateRuntimeHandler(c.runtimeHandler)
		if err != nil {
			return "", err
		}

		return runtimeHandler.RuntimePath, nil
	}

	if !c.trusted {
		if r.untrustedPath != "" {
			return r.untrustedPath, nil
		}

		return r.trustedPath, nil
	}

	// Our container is trusted. Let's look at the configured trust level.
	if r.trustLevel == "trusted" {
		return r.trustedPath, nil
	}

	// Our container is trusted, but we are running untrusted.
	// We will use the untrusted container runtime if it's set
	// and if it's not a privileged container.
	if c.privileged || r.untrustedPath == "" {
		return r.trustedPath, nil
	}

	return r.untrustedPath, nil
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

func (e ExecSyncError) Error() string {
	return fmt.Sprintf("command error: %+v, stdout: %s, stderr: %s, exit code %d", e.Err, e.Stdout.Bytes(), e.Stderr.Bytes(), e.ExitCode)
}

// waitContainerStateStopped runs a loop polling UpdateStatus(), seeking for
// the container status to be updated to 'stopped'. Either it gets the expected
// status and returns nil, or it reaches the timeout and returns an error.
func waitContainerStateStopped(ctx context.Context, c *Container, ri RuntimeImpl, rb RuntimeBase) (err error) {
	// No need to go further and spawn the go routine if the container
	// is already in the expected status.
	if c.State().Status == ContainerStateStopped {
		return nil
	}

	// We need to ensure the container termination will be properly waited
	// for by defining a minimal timeout value. This will prevent timeout
	// value defined in the configuration file to be too low.
	timeout := rb.ctrStopTimeout
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
				if err := ri.UpdateContainerStatus(c); err != nil {
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
