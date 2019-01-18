package oci

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/namespaces"
	client "github.com/containerd/containerd/runtime/v2/shim"
	"github.com/containerd/containerd/runtime/v2/task"
	cio "github.com/containerd/cri/pkg/server/io"
	"github.com/containerd/fifo"
	"github.com/containerd/ttrpc"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"golang.org/x/sys/unix"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	fifoGlobalDir = "/tmp/crio/fifo"
)

// RuntimeVM is the Runtime interface implementation that is more appropriate
// for VM based container runtimes.
type RuntimeVM struct {
	RuntimeBase

	ctx    context.Context
	client *ttrpc.Client
	task   task.TaskService

	ctrs map[string]containerInfo
}

type containerInfo struct {
	cio *cio.ContainerIO
}

// NewRuntimeVM creates a new RuntimeVM instance
func NewRuntimeVM(rb RuntimeBase) (RuntimeImpl, error) {
	return &RuntimeVM{
		RuntimeBase: rb,
		ctx:         context.Background(),
		ctrs:        make(map[string]containerInfo),
	}, nil
}

// CreateContainer creates a container.
func (r *RuntimeVM) CreateContainer(c *Container, cgroupParent string) (err error) {
	logrus.Debug("RuntimeVM.CreateContainer() start")
	defer logrus.Debug("RuntimeVM.CreateContainer() end")

	// Lock the container
	c.opLock.Lock()
	defer c.opLock.Unlock()

	// First thing, we need to start the runtime daemon
	if err = r.startRuntimeDaemon(c); err != nil {
		return err
	}

	// Create IO fifos
	containerIO, err := cio.NewContainerIO(c.ID(),
		cio.WithNewFIFOs(fifoGlobalDir, c.terminal, c.stdin))
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			containerIO.Close()
		}
	}()

	f, err := os.OpenFile(c.LogPath(), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0600)
	if err != nil {
		return err
	}

	containerIO.AddOutput("logfile", f, f)
	containerIO.Pipe()

	r.ctrs[c.ID()] = containerInfo{
		cio: containerIO,
	}

	defer func() {
		if err != nil {
			delete(r.ctrs, c.ID())
		}
	}()

	// We can now create the container, interacting with the server
	request := &task.CreateTaskRequest{
		ID:       c.ID(),
		Bundle:   c.BundlePath(),
		Stdin:    containerIO.Config().Stdin,
		Stdout:   containerIO.Config().Stdout,
		Stderr:   containerIO.Config().Stderr,
		Terminal: containerIO.Config().Terminal,
	}

	createdCh := make(chan error)
	go func() {
		// Create the container
		if _, err := r.task.Create(r.ctx, request); err != nil {
			createdCh <- errdefs.FromGRPC(err)
		}

		close(createdCh)
	}()

	select {
	case err = <-createdCh:
		if err != nil {
			return errors.Errorf("CreateContainer failed: %v", err)
		}
	case <-time.After(ContainerCreateTimeout):
		r.remove(r.ctx, c.ID(), "")
		<-createdCh
		return errors.Errorf("CreateContainer timeout (%v)", ContainerCreateTimeout)
	}

	return nil
}

func (r *RuntimeVM) startRuntimeDaemon(c *Container) error {
	logrus.Debug("RuntimeVM.startRuntimeDaemon() start")
	defer logrus.Debug("RuntimeVM.startRuntimeDaemon() end")

	// Prepare the command to run
	args := []string{"-id", c.ID()}
	if logrus.GetLevel() == logrus.DebugLevel {
		args = append(args, "-debug")
	}
	args = append(args, "start")

	rPath, err := r.path(c)
	if err != nil {
		return err
	}

	// Modify the runtime path so that it complies with v2 shim API
	newRuntimePath := strings.Replace(rPath, "-", ".", -1)

	// Setup default namespace
	r.ctx = namespaces.WithNamespace(r.ctx, namespaces.Default)

	// Prepare the command to exec
	cmd, err := client.Command(
		r.ctx,
		newRuntimePath,
		"",
		c.BundlePath(),
		args...,
	)
	if err != nil {
		return err
	}

	// Create the log file expected by shim-v2 API
	f, err := fifo.OpenFifo(r.ctx, filepath.Join(c.BundlePath(), "log"),
		unix.O_RDONLY|unix.O_CREAT|unix.O_NONBLOCK, 0700)
	if err != nil {
		return err
	}

	// Open the log pipe and block until the writer is ready. This
	// helps with synchronization of the shim. Copy the shim's logs
	// to CRI-O's output.
	go func() {
		defer f.Close()
		if _, err := io.Copy(os.Stderr, f); err != nil {
			logrus.WithError(err).Error("copy shim log")
		}
	}()

	// Start the server
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "%s", out)
	}

	// Retrieve the address from the output
	address := strings.TrimSpace(string(out))

	// Now the RPC server is running, let's connect to it
	conn, err := client.Connect(address, client.AnonDialer)
	if err != nil {
		return err
	}

	cl := ttrpc.NewClient(conn)
	cl.OnClose(func() { conn.Close() })

	// Update the runtime structure
	r.client = cl
	r.task = task.NewTaskClient(cl)

	return nil
}

// StartContainer starts a container.
func (r *RuntimeVM) StartContainer(c *Container) error {
	logrus.Debug("RuntimeVM.StartContainer() start")
	defer logrus.Debug("RuntimeVM.StartContainer() end")

	// Lock the container
	c.opLock.Lock()
	defer c.opLock.Unlock()

	if err := r.start(r.ctx, c.ID(), ""); err != nil {
		return err
	}

	// Spawn a goroutine waiting for the container to terminate. Once it
	// happens, the container status is retrieved to be updated.
	go func() {
		r.wait(r.ctx, c.ID(), "")
		r.UpdateContainerStatus(c)
	}()

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

func (r *RuntimeVM) start(ctx context.Context, ctrID, execID string) error {
	if _, err := r.task.Start(ctx, &task.StartRequest{
		ID:     ctrID,
		ExecID: execID,
	}); err != nil {
		return errdefs.FromGRPC(err)
	}

	return nil
}

func (r *RuntimeVM) wait(ctx context.Context, ctrID, execID string) (int32, time.Time, error) {
	resp, err := r.task.Wait(ctx, &task.WaitRequest{
		ID:     ctrID,
		ExecID: execID,
	})
	if err != nil {
		return -1, time.Time{}, errdefs.FromGRPC(err)
	}

	return int32(resp.ExitStatus), resp.ExitedAt, nil
}

func (r *RuntimeVM) remove(ctx context.Context, ctrID, execID string) error {
	if _, err := r.task.Delete(ctx, &task.DeleteRequest{
		ID:     ctrID,
		ExecID: execID,
	}); err != nil {
		return errdefs.FromGRPC(err)
	}

	return nil
}
