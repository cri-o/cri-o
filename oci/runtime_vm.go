package oci

import (
	"bytes"
	"encoding/hex"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/namespaces"
	client "github.com/containerd/containerd/runtime/v2/shim"
	"github.com/containerd/containerd/runtime/v2/task"
	cioutil "github.com/containerd/cri/pkg/ioutil"
	cio "github.com/containerd/cri/pkg/server/io"
	"github.com/containerd/fifo"
	"github.com/containerd/ttrpc"
	"github.com/containerd/typeurl"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"golang.org/x/sys/unix"
	"k8s.io/client-go/tools/remotecommand"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
	utilexec "k8s.io/utils/exec"
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
	// FIXME: We need to register those types for now, but this should be
	// defined as a specific package that would be shared both by CRI-O and
	// containerd. This would allow shim implementation to import a single
	// package to do the proper registration.
	const prefix = "types.containerd.io"
	// register TypeUrls for commonly marshaled external types
	major := strconv.Itoa(rspec.VersionMajor)
	typeurl.Register(&rspec.Spec{}, prefix, "opencontainers/runtime-spec", major, "Spec")
	typeurl.Register(&rspec.Process{}, prefix, "opencontainers/runtime-spec", major, "Process")
	typeurl.Register(&rspec.LinuxResources{}, prefix, "opencontainers/runtime-spec", major, "LinuxResources")
	typeurl.Register(&rspec.WindowsResources{}, prefix, "opencontainers/runtime-spec", major, "WindowsResources")

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
	logrus.Debug("RuntimeVM.ExecContainer() start")
	defer logrus.Debug("RuntimeVM.ExecContainer() end")

	exitCode, err := r.execContainer(c, cmd, 0, stdin, stdout, stderr, tty, resize)
	if err != nil {
		return err
	}
	if exitCode != 0 {
		return &utilexec.CodeExitError{
			Err:  errors.Errorf("error executing command %v, exit code %d", cmd, exitCode),
			Code: int(exitCode),
		}
	}

	return nil
}

// ExecSyncContainer execs a command in a container and returns it's stdout, stderr and return code.
func (r *RuntimeVM) ExecSyncContainer(c *Container, command []string, timeout int64) (*ExecSyncResponse, error) {
	logrus.Debug("RuntimeVM.ExecSyncContainer() start")
	defer logrus.Debug("RuntimeVM.ExecSyncContainer() end")

	var stdoutBuf, stderrBuf bytes.Buffer
	stdout := cioutil.NewNopWriteCloser(&stdoutBuf)
	stderr := cioutil.NewNopWriteCloser(&stderrBuf)

	exitCode, err := r.execContainer(c, command, timeout, nil, stdout, stderr, c.terminal, nil)
	if err != nil {
		return nil, ExecSyncError{
			ExitCode: -1,
			Err:      errors.Wrapf(err, "ExecSyncContainer failed"),
		}
	}

	return &ExecSyncResponse{
		Stdout:   stdoutBuf.Bytes(),
		Stderr:   stderrBuf.Bytes(),
		ExitCode: exitCode,
	}, nil
}

func (r *RuntimeVM) execContainer(c *Container, cmd []string, timeout int64, stdin io.Reader, stdout, stderr io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) (exitCode int32, err error) {

	logrus.Debug("RuntimeVM.execContainer() start")
	defer logrus.Debug("RuntimeVM.execContainer() end")

	// Cancel the context before returning to ensure goroutines are stopped.
	ctx, cancel := context.WithCancel(r.ctx)
	defer cancel()

	// Generate a unique execID
	execID := generateID()

	// Create IO fifos
	execIO, err := cio.NewExecIO(c.ID(), fifoGlobalDir, tty, stdin != nil)
	if err != nil {
		return -1, errdefs.FromGRPC(err)
	}
	defer execIO.Close()

	execIO.Attach(cio.AttachOptions{
		Stdin:     stdin,
		Stdout:    stdout,
		Stderr:    stderr,
		Tty:       tty,
		StdinOnce: true,
		CloseStdin: func() error {
			return r.closeIO(ctx, c.ID(), execID)
		},
	})

	pSpec := c.Spec().Process
	pSpec.Args = cmd

	any, err := typeurl.MarshalAny(pSpec)
	if err != nil {
		return -1, errdefs.FromGRPC(err)
	}

	request := &task.ExecProcessRequest{
		ID:       c.ID(),
		ExecID:   execID,
		Stdin:    execIO.Config().Stdin,
		Stdout:   execIO.Config().Stdout,
		Stderr:   execIO.Config().Stderr,
		Terminal: execIO.Config().Terminal,
		Spec:     any,
	}

	// Create the "exec" process
	if _, err = r.task.Exec(ctx, request); err != nil {
		return -1, errdefs.FromGRPC(err)
	}

	defer func() {
		if err != nil {
			r.remove(ctx, c.ID(), execID)
		}
	}()

	// Start the process
	if err = r.start(ctx, c.ID(), execID); err != nil {
		return -1, err
	}

	// Initialize terminal resizing if necessary
	if resize != nil {
		kubecontainer.HandleResizing(resize, func(size remotecommand.TerminalSize) {
			logrus.Debugf("Got a resize event: %+v", size)

			if err := r.resizePty(ctx, c.ID(), execID, size); err != nil {
				logrus.Warnf("Failed to resize terminal: %v", err)
			}
		})
	}

	timeoutDuration := time.Duration(timeout) * time.Second

	var timeoutCh <-chan time.Time
	if timeoutDuration == 0 {
		// Do not set timeout if it's 0
		timeoutCh = make(chan time.Time)
	} else {
		timeoutCh = time.After(timeoutDuration)
	}

	execCh := make(chan error)
	go func() {
		// Wait for the process to terminate
		exitCode, _, err = r.wait(ctx, c.ID(), execID)
		if err != nil {
			execCh <- err
		}

		close(execCh)
	}()

	select {
	case err = <-execCh:
		if err != nil {
			r.kill(ctx, c.ID(), execID, syscall.SIGKILL, false)
			return -1, err
		}
	case <-timeoutCh:
		r.kill(ctx, c.ID(), execID, syscall.SIGKILL, false)
		<-execCh
		return -1, errors.Errorf("ExecSyncContainer timeout (%v)", timeoutDuration)
	}

	// Delete the process
	if err := r.remove(ctx, c.ID(), execID); err != nil {
		return -1, err
	}

	return
}

// generateID generates a random unique id.
func generateID() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// UpdateContainer updates container resources
func (r *RuntimeVM) UpdateContainer(c *Container, res *rspec.LinuxResources) error {
	logrus.Debug("RuntimeVM.UpdateContainer() start")
	defer logrus.Debug("RuntimeVM.UpdateContainer() end")

	// Lock the container
	c.opLock.Lock()
	defer c.opLock.Unlock()

	// Convert resources into protobuf Any type
	any, err := typeurl.MarshalAny(res)
	if err != nil {
		return err
	}

	if _, err := r.task.Update(r.ctx, &task.UpdateTaskRequest{
		ID:        c.ID(),
		Resources: any,
	}); err != nil {
		return errdefs.FromGRPC(err)
	}

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

func (r *RuntimeVM) kill(ctx context.Context, ctrID, execID string, signal syscall.Signal, all bool) error {
	if _, err := r.task.Kill(ctx, &task.KillRequest{
		ID:     ctrID,
		ExecID: execID,
		Signal: uint32(signal),
		All:    all,
	}); err != nil {
		return errdefs.FromGRPC(err)
	}

	return nil
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

func (r RuntimeVM) resizePty(ctx context.Context, ctrID, execID string, size remotecommand.TerminalSize) error {
	_, err := r.task.ResizePty(ctx, &task.ResizePtyRequest{
		ID:     ctrID,
		ExecID: execID,
		Width:  uint32(size.Width),
		Height: uint32(size.Height),
	})
	if err != nil {
		return errdefs.FromGRPC(err)
	}

	return nil
}

func (r *RuntimeVM) closeIO(ctx context.Context, ctrID, execID string) error {
	_, err := r.task.CloseIO(ctx, &task.CloseIORequest{
		ID:     ctrID,
		ExecID: execID,
		Stdin:  true,
	})
	if err != nil {
		return errdefs.FromGRPC(err)
	}

	return nil
}
