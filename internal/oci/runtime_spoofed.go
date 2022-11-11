package oci

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
	"time"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/pkg/config"
	"github.com/cri-o/cri-o/utils/cmdrunner"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"k8s.io/client-go/tools/remotecommand"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)
// runtimeSpoofed is the Runtime interface implementation relying on conmon to
// interact with the container runtime.
type runtimeSpoofed struct {
	*Runtime

	root    string
	handler *config.RuntimeHandler
}

// newRuntimeSpoofed creates a new runtimeSpoofed instance
func newRuntimeSpoofed(r *Runtime, handler *config.RuntimeHandler) RuntimeImpl {
	runRoot := config.DefaultRuntimeRoot
	if handler.RuntimeRoot != "" {
		runRoot = handler.RuntimeRoot
	}

	return &runtimeSpoofed{
		Runtime: r,
		root:    runRoot,
		handler: handler,
	}
}


// CreateContainer creates a container.
func (r *runtimeSpoofed) CreateContainer(ctx context.Context, c *Container, cgroupParent string, restore bool) (retErr error) {

	// Start sleep command to create process space for container
	cmd := cmdrunner.Command("/bin/sleep","infinity") // nolint: gosec
	cmd.Dir = c.bundlePath
	cmd.SysProcAttr = sysProcAttrPlatform()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if c.terminal {
		var stderrBuf bytes.Buffer
		cmd.Stderr = &stderrBuf
	}
	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("error creating spoofed process: %v", err)
	}
	pid := cmd.Process.Pid
	logrus.Infof("Setting spoofed Pid to %d", pid)
	if err := c.state.SetInitPid(pid); err != nil {
		return err
	}
	return nil
}

// StartContainer starts a container.
func (r *runtimeSpoofed) StartContainer(ctx context.Context, c *Container) error {
	_, span := log.StartSpan(ctx)
	defer span.End()
	c.opLock.Lock()
	defer c.opLock.Unlock()

	c.state.Started = time.Now()
	return nil
}

// ExecContainer prepares a streaming endpoint to execute a command in the container.
func (r *runtimeSpoofed) ExecContainer(ctx context.Context, c *Container, cmd []string, stdin io.Reader, stdout, stderr io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) error {
	_, span := log.StartSpan(ctx)
	defer span.End()

	log.Debugf(ctx, "Can't Exec Spoofed Container")
	return nil
}

// ExecSyncContainer execs a command in a container and returns it's stdout, stderr and return code.
func (r *runtimeSpoofed) ExecSyncContainer(ctx context.Context, c *Container, command []string, timeout int64) (*types.ExecSyncResponse, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	log.Debugf(ctx, "Can't ExecSync Spoofed Container")
	return &types.ExecSyncResponse{
		Stdout:   []byte{},
		Stderr:   []byte{},
		ExitCode: 0,
	}, nil
}

// UpdateContainer updates container resources
func (r *runtimeSpoofed) UpdateContainer(ctx context.Context, c *Container, res *rspec.LinuxResources) error {
	_, span := log.StartSpan(ctx)
	defer span.End()

	c.opLock.Lock()
	defer c.opLock.Unlock()

	return nil
}

// StopContainer stops a container. Timeout is given in seconds.
func (r *runtimeSpoofed) StopContainer(ctx context.Context, c *Container, timeout int64) (retErr error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	if c.SetAsStopping(timeout) {
		return nil
	}
	defer func() {
		// Failed to stop, set stopping to false.
		// Otherwise, we won't actually
		// attempt to stop when a new request comes in,
		// even though we're not actively stopping anymore.
		// Also, close the stopStoppingChan to tell
		// routines waiting to change the stop timeout to give up.
		close(c.stopStoppingChan)
		c.SetAsNotStopping()
	}()

	c.opLock.Lock()
	defer c.opLock.Unlock()

	if err := c.ShouldBeStopped(); err != nil {
		return err
	}

	c.state.Status = ContainerStateStopped
	c.state.Finished = time.Now()
	return nil
}

// DeleteContainer deletes a container.
func (r *runtimeSpoofed) DeleteContainer(ctx context.Context, c *Container) error {
	_, span := log.StartSpan(ctx)
	defer span.End()
	c.opLock.Lock()
	defer c.opLock.Unlock()

	return nil
}

// UpdateContainerStatus refreshes the status of the container.
func (r *runtimeSpoofed) UpdateContainerStatus(ctx context.Context, c *Container) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	c.opLock.Lock()
	defer c.opLock.Unlock()

	status := c.state.Status
	if c.state.InitPid != 0 && status == "" {
		status = ContainerStateCreated
	} else if !c.state.Started.IsZero() && status == ContainerStateCreated  {
		status = ContainerStateRunning
	} 
	c.state.Status = status
	return nil
}

// PauseContainer pauses a container.
func (r *runtimeSpoofed) PauseContainer(ctx context.Context, c *Container) error {
	c.opLock.Lock()
	defer c.opLock.Unlock()

	return nil
}

// UnpauseContainer unpauses a container.
func (r *runtimeSpoofed) UnpauseContainer(ctx context.Context, c *Container) error {
	c.opLock.Lock()
	defer c.opLock.Unlock()

	return nil
}

// ContainerStats provides statistics of a container.
func (r *runtimeSpoofed) ContainerStats(ctx context.Context, c *Container, cgroup string) (*types.ContainerStats, error) {
	_, span := log.StartSpan(ctx)
	defer span.End()
	c.opLock.Lock()
	defer c.opLock.Unlock()


	stats := &types.ContainerStats{
		Attributes: c.CRIAttributes(),
	}
	return stats, nil
}

// SignalContainer sends a signal to a container process.
func (r *runtimeSpoofed) SignalContainer(ctx context.Context, c *Container, sig syscall.Signal) error {
	_, span := log.StartSpan(ctx)
	defer span.End()
	c.opLock.Lock()
	defer c.opLock.Unlock()

	return nil
}

// AttachContainer attaches IO to a running container.
func (r *runtimeSpoofed) AttachContainer(ctx context.Context, c *Container, inputStream io.Reader, outputStream, errorStream io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	return errors.New("Can't Attach IO to Spoofed Container")
}

// PortForwardContainer forwards the specified port into the provided container.
func (r *runtimeSpoofed) PortForwardContainer(ctx context.Context, c *Container, netNsPath string, port int32, stream io.ReadWriteCloser) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	return errors.New("Can't Port Forward Spoofed Container")
}

// ReopenContainerLog reopens the log file of a container.
func (r *runtimeSpoofed) ReopenContainerLog(ctx context.Context, c *Container) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	return nil
}

// CheckpointContainer checkpoints a container.
func (r *runtimeSpoofed) CheckpointContainer(ctx context.Context, c *Container, specgen *rspec.Spec, leaveRunning bool) error {
	c.opLock.Lock()
	defer c.opLock.Unlock()

	return fmt.Errorf("configured runtime does not support checkpoint/restore")
}

// RestoreContainer restores a container.
func (r *runtimeSpoofed) RestoreContainer(ctx context.Context, c *Container, sbSpec *rspec.Spec, infraPid int, cgroupParent string) error {
	return fmt.Errorf("configured runtime does not support checkpoint/restore")
}