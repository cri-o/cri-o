package oci

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/containerd/namespaces"
	client "github.com/containerd/containerd/runtime/v2/shim"
	"github.com/containerd/containerd/runtime/v2/task"
	"github.com/containerd/ttrpc"
	"github.com/containers/libpod/pkg/cgroups"
	"github.com/cri-o/cri-o/utils"
	"github.com/cri-o/cri-o/utils/errdefs"
	"github.com/cri-o/cri-o/utils/fifo"
	cio "github.com/cri-o/cri-o/utils/io"
	cioutil "github.com/cri-o/cri-o/utils/ioutil"
	"github.com/cri-o/cri-o/utils/typeurl"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"golang.org/x/sys/unix"
	"k8s.io/client-go/tools/remotecommand"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
	utilexec "k8s.io/utils/exec"
)

const fifoGlobalDir = "/tmp/crio/fifo"

// runtimeVM is the Runtime interface implementation that is more appropriate
// for VM based container runtimes.
type runtimeVM struct {
	path string

	ctx    context.Context
	client *ttrpc.Client
	task   task.TaskService

	ctrs map[string]containerInfo
}

type containerInfo struct {
	cio *cio.ContainerIO
}

// newRuntimeVM creates a new runtimeVM instance
func newRuntimeVM(path string) RuntimeImpl {
	logrus.Debug("oci.newRuntimeVM() start")
	defer logrus.Debug("oci.newRuntimeVM() end")

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

	return &runtimeVM{
		path: path,
		ctx:  context.Background(),
		ctrs: make(map[string]containerInfo),
	}
}

// CreateContainer creates a container.
func (r *runtimeVM) CreateContainer(c *Container, cgroupParent string) (err error) {
	logrus.Debug("runtimeVM.createContainer() start")
	defer logrus.Debug("runtimeVM.createContainer() end")

	// Lock the container
	c.opLock.Lock()
	defer c.opLock.Unlock()

	// First thing, we need to start the runtime daemon
	if err := r.startRuntimeDaemon(c); err != nil {
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
		if err := r.remove(r.ctx, c.ID(), ""); err != nil {
			return err
		}
		<-createdCh
		return errors.Errorf("CreateContainer timeout (%v)", ContainerCreateTimeout)
	}

	return nil
}

func (r *runtimeVM) startRuntimeDaemon(c *Container) error {
	logrus.Debug("runtimeVM.startRuntimeDaemon() start")
	defer logrus.Debug("runtimeVM.startRuntimeDaemon() end")

	// Prepare the command to run
	args := []string{"-id", c.ID()}
	if logrus.GetLevel() == logrus.DebugLevel {
		args = append(args, "-debug")
	}
	args = append(args, "start")

	// Modify the runtime path so that it complies with v2 shim API
	newRuntimePath := strings.Replace(r.path, "-", ".", -1) // nolint: gocritic

	// Setup default namespace
	r.ctx = namespaces.WithNamespace(r.ctx, namespaces.Default)

	// Prepare the command to exec
	cmd, err := client.Command(
		r.ctx,
		newRuntimePath,
		"",
		"",
		c.BundlePath(),
		nil,
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

	options := ttrpc.WithOnClose(func() { conn.Close() })
	cl := ttrpc.NewClient(conn, options)

	// Update the runtime structure
	r.client = cl
	r.task = task.NewTaskClient(cl)

	return nil
}

// StartContainer starts a container.
func (r *runtimeVM) StartContainer(c *Container) error {
	logrus.Debug("runtimeVM.startContainer() start")
	defer logrus.Debug("runtimeVM.startContainer() end")

	// Lock the container
	c.opLock.Lock()
	defer c.opLock.Unlock()

	if err := r.start(r.ctx, c.ID(), ""); err != nil {
		return err
	}

	// Spawn a goroutine waiting for the container to terminate. Once it
	// happens, the container status is retrieved to be updated.
	var err error
	go func() {
		_, _, err = r.wait(r.ctx, c.ID(), "")
		if err == nil {
			err = r.UpdateContainerStatus(c)
		}
	}()

	return errors.Wrapf(err, "start container")
}

// ExecContainer prepares a streaming endpoint to execute a command in the container.
func (r *runtimeVM) ExecContainer(c *Container, cmd []string, stdin io.Reader, stdout, stderr io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) error {
	logrus.Debug("runtimeVM.execContainer() start")
	defer logrus.Debug("runtimeVM.execContainer() end")

	exitCode, err := r.execContainerCommon(c, cmd, 0, stdin, stdout, stderr, tty, resize)
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
func (r *runtimeVM) ExecSyncContainer(c *Container, command []string, timeout int64) (*ExecSyncResponse, error) {
	logrus.Debug("runtimeVM.execSyncContainer() start")
	defer logrus.Debug("runtimeVM.execSyncContainer() end")

	var stdoutBuf, stderrBuf bytes.Buffer
	stdout := cioutil.NewNopWriteCloser(&stdoutBuf)
	stderr := cioutil.NewNopWriteCloser(&stderrBuf)

	exitCode, err := r.execContainerCommon(c, command, timeout, nil, stdout, stderr, c.terminal, nil)
	if err != nil {
		return nil, &ExecSyncError{
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

func (r *runtimeVM) execContainerCommon(c *Container, cmd []string, timeout int64, stdin io.Reader, stdout, stderr io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) (exitCode int32, err error) {

	logrus.Debug("runtimeVM.execContainer() start")
	defer logrus.Debug("runtimeVM.execContainer() end")

	// Cancel the context before returning to ensure goroutines are stopped.
	ctx, cancel := context.WithCancel(r.ctx)
	defer cancel()

	// Generate a unique execID
	execID, err := utils.GenerateID()
	if err != nil {
		return -1, errors.Wrapf(err, "exec container")
	}

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
			if err := r.remove(ctx, c.ID(), execID); err != nil {
				logrus.Debugf("unable to remove container %s: %v", c.ID(), err)
			}
		}
	}()

	// Start the process
	if err := r.start(ctx, c.ID(), execID); err != nil {
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
			if killErr := r.kill(ctx, c.ID(), execID, syscall.SIGKILL, false); killErr != nil {
				return -1, killErr
			}
			return -1, err
		}
	case <-timeoutCh:
		if killErr := r.kill(ctx, c.ID(), execID, syscall.SIGKILL, false); killErr != nil {
			return -1, killErr
		}
		<-execCh
		return -1, errors.Errorf("ExecSyncContainer timeout (%v)", timeoutDuration)
	}

	// Delete the process
	if err := r.remove(ctx, c.ID(), execID); err != nil {
		return -1, err
	}

	return exitCode, err
}

// UpdateContainer updates container resources
func (r *runtimeVM) UpdateContainer(c *Container, res *rspec.LinuxResources) error {
	logrus.Debug("runtimeVM.updateContainer() start")
	defer logrus.Debug("runtimeVM.updateContainer() end")

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

// StopContainer stops a container. Timeout is given in seconds.
func (r *runtimeVM) StopContainer(ctx context.Context, c *Container, timeout int64) error {
	logrus.Debug("runtimeVM.StopContainer() start")
	defer logrus.Debug("runtimeVM.StopContainer() end")

	// Lock the container
	c.opLock.Lock()
	defer c.opLock.Unlock()

	// Cancel the context before returning to ensure goroutines are stopped.
	ctx, cancel := context.WithCancel(r.ctx)
	defer cancel()

	stopCh := make(chan error)
	go func() {
		if _, _, err := r.wait(ctx, c.ID(), ""); err != nil {
			stopCh <- errdefs.FromGRPC(err)
		}

		close(stopCh)
	}()

	var sig syscall.Signal

	if timeout > 0 {
		sig = c.StopSignal()
		// Send a stopping signal to the container
		if err := r.kill(ctx, c.ID(), "", sig, false); err != nil {
			return err
		}

		timeoutDuration := time.Duration(timeout) * time.Second

		err := r.waitCtrTerminate(sig, stopCh, timeoutDuration)
		if err == nil {
			return nil
		}
		logrus.Warnf("%v", err)
	}

	sig = syscall.SIGKILL
	// Send a SIGKILL signal to the container
	if err := r.kill(ctx, c.ID(), "", sig, false); err != nil {
		return err
	}

	if err := r.waitCtrTerminate(sig, stopCh, killContainerTimeout); err != nil {
		logrus.Errorf("%v", err)
		return err
	}

	return nil
}

func (r *runtimeVM) waitCtrTerminate(sig syscall.Signal, stopCh chan error, timeout time.Duration) error {
	select {
	case err := <-stopCh:
		return err
	case <-time.After(timeout):
		return errors.Errorf("StopContainer with signal %v timed out after (%v)", sig, timeout)
	}
}

// DeleteContainer deletes a container.
func (r *runtimeVM) DeleteContainer(c *Container) error {
	logrus.Debug("runtimeVM.DeleteContainer() start")
	defer logrus.Debug("runtimeVM.DeleteContainer() end")

	// Lock the container
	c.opLock.Lock()
	defer c.opLock.Unlock()

	cInfo, ok := r.ctrs[c.ID()]
	if !ok {
		return errors.New("Could not retrieve container information")
	}

	if err := cInfo.cio.Close(); err != nil {
		return err
	}

	if err := r.remove(r.ctx, c.ID(), ""); err != nil {
		return err
	}

	if _, err := r.task.Shutdown(r.ctx, &task.ShutdownRequest{ID: c.ID()}); err != nil {
		return err
	}

	delete(r.ctrs, c.ID())

	return nil
}

// UpdateContainerStatus refreshes the status of the container.
func (r *runtimeVM) UpdateContainerStatus(c *Container) error {
	logrus.Debug("runtimeVM.UpdateContainerStatus() start")
	defer logrus.Debug("runtimeVM.UpdateContainerStatus() end")

	// Lock the container
	c.opLock.Lock()
	defer c.opLock.Unlock()

	// This can happen on restore, for example if we switch the runtime type
	// for a container from "oci" to "vm" for the same runtime.
	if r.task == nil {
		return errors.New("runtime not correctly setup")
	}

	response, err := r.task.State(r.ctx, &task.StateRequest{
		ID: c.ID(),
	})
	if err != nil {
		if errors.Cause(err) != ttrpc.ErrClosed {
			return errdefs.FromGRPC(err)
		}
		return errdefs.ErrNotFound
	}

	status := c.state.Status
	switch response.Status {
	case tasktypes.StatusCreated:
		status = ContainerStateCreated
	case tasktypes.StatusRunning:
		status = ContainerStateRunning
	case tasktypes.StatusStopped:
		status = ContainerStateStopped
	case tasktypes.StatusPaused:
		status = ContainerStatePaused
	}

	c.state.Status = status
	c.state.Finished = response.ExitedAt
	exitCode := int32(response.ExitStatus)
	c.state.ExitCode = &exitCode

	return nil
}

// PauseContainer pauses a container.
func (r *runtimeVM) PauseContainer(c *Container) error {
	logrus.Debug("runtimeVM.PauseContainer() start")
	defer logrus.Debug("runtimeVM.PauseContainer() end")

	// Lock the container
	c.opLock.Lock()
	defer c.opLock.Unlock()

	if _, err := r.task.Pause(r.ctx, &task.PauseRequest{
		ID: c.ID(),
	}); err != nil {
		return errdefs.FromGRPC(err)
	}

	return nil
}

// UnpauseContainer unpauses a container.
func (r *runtimeVM) UnpauseContainer(c *Container) error {
	logrus.Debug("runtimeVM.UnpauseContainer() start")
	defer logrus.Debug("runtimeVM.UnpauseContainer() end")

	// Lock the container
	c.opLock.Lock()
	defer c.opLock.Unlock()

	if _, err := r.task.Resume(r.ctx, &task.ResumeRequest{
		ID: c.ID(),
	}); err != nil {
		return errdefs.FromGRPC(err)
	}

	return nil
}

// ContainerStats provides statistics of a container.
func (r *runtimeVM) ContainerStats(c *Container) (*ContainerStats, error) {
	logrus.Debug("runtimeVM.ContainerStats() start")
	defer logrus.Debug("runtimeVM.ContainerStats() end")

	// Lock the container with a shared lock
	c.opLock.RLock()
	defer c.opLock.RUnlock()

	resp, err := r.task.Stats(r.ctx, &task.StatsRequest{
		ID: c.ID(),
	})
	if err != nil {
		return nil, errdefs.FromGRPC(err)
	}
	if resp == nil {
		return nil, errors.New("Could not retrieve container stats")
	}

	stats, err := typeurl.UnmarshalAny(resp.Stats)
	if err != nil {
		return nil, errors.Wrap(err, "failed to extract container metrics")
	}

	metrics, ok := stats.(*cgroups.Metrics)
	if !ok {
		return nil, errors.Errorf("Unknown stats type %T", stats)
	}

	return metricsToCtrStats(c, metrics), nil
}

// SignalContainer sends a signal to a container process.
func (r *runtimeVM) SignalContainer(c *Container, sig syscall.Signal) error {
	logrus.Debug("runtimeVM.SignalContainer() start")
	defer logrus.Debug("runtimeVM.SignalContainer() end")

	// Lock the container
	c.opLock.Lock()
	defer c.opLock.Unlock()

	return r.kill(r.ctx, c.ID(), "", sig, true)
}

// AttachContainer attaches IO to a running container.
func (r *runtimeVM) AttachContainer(c *Container, inputStream io.Reader, outputStream, errorStream io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) error {
	logrus.Debug("runtimeVM.AttachContainer() start")
	defer logrus.Debug("runtimeVM.AttachContainer() end")

	// Initialize terminal resizing
	kubecontainer.HandleResizing(resize, func(size remotecommand.TerminalSize) {
		logrus.Debugf("Got a resize event: %+v", size)

		if err := r.resizePty(r.ctx, c.ID(), "", size); err != nil {
			logrus.Warnf("Failed to resize terminal: %v", err)
		}
	})

	cInfo, ok := r.ctrs[c.ID()]
	if !ok {
		return errors.New("Could not retrieve container information")
	}

	opts := cio.AttachOptions{
		Stdin:     inputStream,
		Stdout:    outputStream,
		Stderr:    errorStream,
		Tty:       tty,
		StdinOnce: c.stdinOnce,
		CloseStdin: func() error {
			return r.closeIO(r.ctx, c.ID(), "")
		},
	}

	return cInfo.cio.Attach(opts)
}

// PortForwardContainer forwards the specified port provides statistics of a container.
func (r *runtimeVM) PortForwardContainer(c *Container, port int32, stream io.ReadWriter) error {
	logrus.Debug("runtimeVM.PortForwardContainer() start")
	defer logrus.Debug("runtimeVM.PortForwardContainer() end")

	return nil
}

// ReopenContainerLog reopens the log file of a container.
func (r *runtimeVM) ReopenContainerLog(c *Container) error {
	logrus.Debug("runtimeVM.ReopenContainerLog() start")
	defer logrus.Debug("runtimeVM.ReopenContainerLog() end")

	return nil
}

func (r *runtimeVM) WaitContainerStateStopped(ctx context.Context, c *Container) error {
	return nil
}

func (r *runtimeVM) start(ctx context.Context, ctrID, execID string) error {
	if _, err := r.task.Start(ctx, &task.StartRequest{
		ID:     ctrID,
		ExecID: execID,
	}); err != nil {
		return errdefs.FromGRPC(err)
	}

	return nil
}

func (r *runtimeVM) wait(ctx context.Context, ctrID, execID string) (int32, time.Time, error) {
	resp, err := r.task.Wait(ctx, &task.WaitRequest{
		ID:     ctrID,
		ExecID: execID,
	})
	if err != nil {
		return -1, time.Time{}, errdefs.FromGRPC(err)
	}

	return int32(resp.ExitStatus), resp.ExitedAt, nil
}

func (r *runtimeVM) kill(ctx context.Context, ctrID, execID string, signal syscall.Signal, all bool) error {
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

func (r *runtimeVM) remove(ctx context.Context, ctrID, execID string) error {
	if _, err := r.task.Delete(ctx, &task.DeleteRequest{
		ID:     ctrID,
		ExecID: execID,
	}); err != nil {
		return errdefs.FromGRPC(err)
	}

	return nil
}

func (r runtimeVM) resizePty(ctx context.Context, ctrID, execID string, size remotecommand.TerminalSize) error {
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

func (r *runtimeVM) closeIO(ctx context.Context, ctrID, execID string) error {
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
