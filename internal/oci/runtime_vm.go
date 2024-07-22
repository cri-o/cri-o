package oci

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	cgroupsV1 "github.com/containerd/cgroups/stats/v1"
	cgroupsV2 "github.com/containerd/cgroups/v2/stats"
	"github.com/containerd/containerd/api/runtime/task/v2"
	containerdTypes "github.com/containerd/containerd/api/types"
	tasktypes "github.com/containerd/containerd/api/types/task"
	ctrio "github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/namespaces"
	cio "github.com/containerd/containerd/pkg/cri/io"
	cioutil "github.com/containerd/containerd/pkg/ioutil"
	"github.com/containerd/containerd/protobuf"
	client "github.com/containerd/containerd/runtime/v2/shim"
	runtimeoptions "github.com/containerd/cri-containerd/pkg/api/runtimeoptions/v1"
	"github.com/containerd/fifo"
	"github.com/containerd/ttrpc"
	"github.com/containerd/typeurl"
	conmonconfig "github.com/containers/conmon/runner/config"
	"github.com/cri-o/cri-o/internal/config/cgmgr"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/pkg/annotations"
	"github.com/cri-o/cri-o/pkg/config"
	"github.com/cri-o/cri-o/server/metrics"
	"github.com/cri-o/cri-o/utils"
	"github.com/cri-o/cri-o/utils/errdefs"
	katavolume "github.com/kata-containers/kata-containers/src/runtime/virtcontainers/types"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"golang.org/x/sys/unix"
	anypb "google.golang.org/protobuf/types/known/anypb"
	"k8s.io/client-go/tools/remotecommand"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	utilexec "k8s.io/utils/exec"
)

// runtimeVM is the Runtime interface implementation that is more appropriate
// for VM based container runtimes.
type runtimeVM struct {
	path       string
	fifoDir    string
	configPath string
	exitsPath  string
	pullImage  bool
	ctx        context.Context
	client     *ttrpc.Client
	task       task.TaskService

	sync.Mutex
	ctrs map[string]containerInfo
}

type containerInfo struct {
	cio *cio.ContainerIO
}

const (
	KataVirtualVolumeOptionName = "io.katacontainers.volume"
)

const (
	execError   = -1
	execTimeout = -2
)

// newRuntimeVM creates a new runtimeVM instance
func newRuntimeVM(handler *config.RuntimeHandler, exitsPath string) RuntimeImpl {
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
		path:       handler.RuntimePath,
		configPath: handler.RuntimeConfigPath,
		exitsPath:  exitsPath,
		pullImage:  handler.RuntimePullImage,
		fifoDir:    filepath.Join(handler.RuntimeRoot, "crio", "fifo"),
		ctx:        context.Background(),
		ctrs:       make(map[string]containerInfo),
	}
}

func addVolumeMountsToCreateRequest(ctx context.Context, request *task.CreateTaskRequest, c *Container) error {
	// To make the kata agent pull the image{"volume_type":"image_guest_pull","source":"quay.io/kata-containers/confidential-containers:unsigned","fs_type":"overlayfs","image_pull":{"metadata":{}}}

	imageSource := c.Spec().Annotations[annotations.ImageName]
	log.Infof(ctx, "Adding mount info to pull image %s", imageSource)

	volume := &katavolume.KataVirtualVolume{
		VolumeType: katavolume.KataVirtualVolumeImageGuestPullType,
		Source:     imageSource,
		FSType:     "overlay_fs",
		ImagePull:  &katavolume.ImagePullVolume{Metadata: c.Spec().Annotations},
	}
	if !volume.IsValid() {
		return fmt.Errorf("got invalid kata volume, %v", volume)
	}

	info, err := EncodeKataVirtualVolumeToBase64(ctx, volume)
	if err != nil {
		return fmt.Errorf("failed to encode Kata Volume info %v: %w", volume, err)
	}

	log.Infof(ctx, "CoCo : Mount volume information: %v (encoded: %v)", volume, info)

	opt := fmt.Sprintf("%s=%s", KataVirtualVolumeOptionName, info)

	request.Rootfs = append(request.Rootfs, &containerdTypes.Mount{
		// we don't actually use nydus, but this string is what makes the kata
		// shim check for the extended options and ignore the mount, passing the
		// information to the kata agent instead.
		Type:    "fuse.nydus-overlayfs",
		Options: []string{opt},
	})
	return nil
}

// CreateContainer creates a container.
func (r *runtimeVM) CreateContainer(ctx context.Context, c *Container, cgroupParent string, restore bool) (retErr error) {
	log.Debugf(ctx, "RuntimeVM.CreateContainer() start")
	defer log.Debugf(ctx, "RuntimeVM.CreateContainer() end")

	// Lock the container
	c.opLock.Lock()
	defer c.opLock.Unlock()

	// Lets ensure we're able to properly get construct the Options
	// that we'll pass to the ContainerCreateTask, as admins can set
	// the runtime_config_path to an arbitrary location.  Also, lets
	// fail early if something goes wrong.
	var opts *anypb.Any = nil
	if r.configPath != "" {
		runtimeOptions := &runtimeoptions.Options{
			ConfigPath: r.configPath,
		}

		marshaledOtps, err := typeurl.MarshalAny(runtimeOptions)
		if err != nil {
			return err
		}

		opts = protobuf.FromAny(marshaledOtps)
	}

	// First thing, we need to start the runtime daemon
	if err := r.startRuntimeDaemon(ctx, c); err != nil {
		return err
	}

	containerIO, err := r.createContainerIO(ctx, c, cio.WithNewFIFOs(r.fifoDir, c.terminal, c.stdin))
	if err != nil {
		return err
	}

	defer func() {
		if retErr != nil {
			log.Warnf(ctx, "Cleaning up container %s: %v", c.ID(), retErr)
			if cleanupErr := r.deleteContainer(c, true); cleanupErr != nil {
				log.Infof(ctx, "DeleteContainer failed for container %s: %v", c.ID(), cleanupErr)
			}
			if err := os.Remove(c.logPath); err != nil {
				log.Warnf(ctx, "Failed to remove log path %s after failing to create container: %v", c.logPath, err)
			}
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
		Options:  opts,
	}

	if r.pullImage {
		err := addVolumeMountsToCreateRequest(ctx, request, c)
		if err != nil {
			log.Warnf(ctx, "Failed to add KataVirtualVolume information to CreateContainer: %v", err)
		}
	}

	createdCh := make(chan error)
	go func() {
		// Create the container
		if resp, err := r.task.Create(r.ctx, request); err != nil {
			createdCh <- errdefs.FromGRPC(err)
		} else if err := c.state.SetInitPid(int(resp.Pid)); err != nil {
			createdCh <- err
		}

		close(createdCh)
	}()

	select {
	case err = <-createdCh:
		if err != nil {
			return fmt.Errorf("CreateContainer failed: %w", err)
		}
	case <-time.After(ContainerCreateTimeout):
		if err := r.remove(c.ID(), ""); err != nil {
			return err
		}
		<-createdCh
		return fmt.Errorf("CreateContainer timeout (%v)", ContainerCreateTimeout)
	}

	return nil
}

func (r *runtimeVM) startRuntimeDaemon(ctx context.Context, c *Container) error {
	log.Debugf(ctx, "RuntimeVM.startRuntimeDaemon() start")
	defer log.Debugf(ctx, "RuntimeVM.startRuntimeDaemon() end")

	// Prepare the command to run
	args := []string{"-id", c.ID()}
	switch logrus.GetLevel() {
	case logrus.DebugLevel, logrus.TraceLevel:
		args = append(args, "-debug")
	}
	args = append(args, "start")

	r.ctx = namespaces.WithNamespace(r.ctx, namespaces.Default)

	// Prepare the command to exec
	cmd, err := client.Command(
		r.ctx,
		&client.CommandConfig{
			Runtime: r.path,
			Path:    c.BundlePath(),
			Args:    args,
		},
	)
	if err != nil {
		return err
	}

	// Create the log file expected by shim-v2 API
	f, err := fifo.OpenFifo(r.ctx, filepath.Join(c.BundlePath(), "log"),
		unix.O_RDONLY|unix.O_CREAT|unix.O_NONBLOCK, 0o700)
	if err != nil {
		return err
	}

	// Open the log pipe and block until the writer is ready. This
	// helps with synchronization of the shim. Copy the shim's logs
	// to CRI-O's output.
	go func() {
		defer f.Close()
		if _, err := io.Copy(os.Stderr, f); err != nil {
			log.Errorf(ctx, "Copy shim log: %v", err)
		}
	}()

	// Start the server
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", string(out), err)
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
func (r *runtimeVM) StartContainer(ctx context.Context, c *Container) error {
	log.Debugf(ctx, "RuntimeVM.StartContainer() start")
	defer log.Debugf(ctx, "RuntimeVM.StartContainer() end")

	// Lock the container
	c.opLock.Lock()
	defer c.opLock.Unlock()

	if err := r.start(c.ID(), ""); err != nil {
		return err
	}
	c.state.Started = time.Now()

	// Spawn a goroutine waiting for the container to terminate. Once it
	// happens, the container status is retrieved to be updated.
	go func() {
		_, err := r.wait(c.ID(), "")
		if err == nil {
			// create a file on the exitsDir so that cri-o server can detect it
			path := filepath.Join(r.exitsPath+"/", c.ID())
			if fileErr := os.WriteFile(path, []byte("Exited"), 0o644); fileErr != nil {
				log.Warnf(ctx, "Unable to write exit file %v", fileErr)
			}
			if err1 := r.updateContainerStatus(ctx, c); err1 != nil {
				log.Warnf(ctx, "Error updating container status %v", err1)
			}
		} else {
			log.Warnf(ctx, "Wait for %s returned: %v", c.ID(), err)
		}
	}()

	return nil
}

// ExecContainer prepares a streaming endpoint to execute a command in the container.
func (r *runtimeVM) ExecContainer(ctx context.Context, c *Container, cmd []string, stdin io.Reader, stdout, stderr io.WriteCloser, tty bool, resizeChan <-chan remotecommand.TerminalSize) error {
	log.Debugf(ctx, "RuntimeVM.ExecContainer() start")
	defer log.Debugf(ctx, "RuntimeVM.ExecContainer() end")

	exitCode, err := r.execContainerCommon(ctx, c, cmd, 0, stdin, stdout, stderr, tty, resizeChan)
	if err != nil {
		return err
	}
	if exitCode != 0 {
		return &utilexec.CodeExitError{
			Err:  fmt.Errorf("executing command %v, exit code %d", cmd, exitCode),
			Code: int(exitCode),
		}
	}

	return nil
}

// writeCloserWrapper represents a WriteCloser whose closer operation is noop.
type writeCloserWrapper struct {
	Writer io.Writer
}

func (w *writeCloserWrapper) Write(buf []byte) (int, error) {
	return w.Writer.Write(buf)
}

func (w *writeCloserWrapper) Close() error {
	return nil
}

// ExecSyncContainer execs a command in a container and returns it's stdout, stderr and return code.
func (r *runtimeVM) ExecSyncContainer(ctx context.Context, c *Container, command []string, timeout int64) (*types.ExecSyncResponse, error) {
	log.Debugf(ctx, "RuntimeVM.ExecSyncContainer() start")
	defer log.Debugf(ctx, "RuntimeVM.ExecSyncContainer() end")

	var stdoutBuf, stderrBuf bytes.Buffer
	stdout := &writeCloserWrapper{limitWriter(&stdoutBuf, maxExecSyncSize)}
	stderr := &writeCloserWrapper{limitWriter(&stderrBuf, maxExecSyncSize)}

	exitCode, err := r.execContainerCommon(ctx, c, command, timeout, nil, stdout, stderr, c.terminal, nil)
	if err != nil {
		return nil, fmt.Errorf("ExecSyncContainer failed: %w", err)
	}

	// if the execution stopped because of the timeout, report it as such
	if exitCode == execTimeout {
		return &types.ExecSyncResponse{
			Stderr:   []byte(conmonconfig.TimedOutMessage),
			ExitCode: -1,
		}, nil
	}

	return &types.ExecSyncResponse{
		Stdout:   stdoutBuf.Bytes(),
		Stderr:   stderrBuf.Bytes(),
		ExitCode: exitCode,
	}, nil
}

// limitWriter is a copy of the standard library ioutils.LimitReader,
// applied to the writer interface.
// limitWriter returns a Writer that writes to w
// but stops with EOF after n bytes.
// The underlying implementation is a *LimitedWriter.
func limitWriter(w io.Writer, n int64) io.Writer { return &limitedWriter{w, n} }

// A limitedWriter writes to W but limits the amount of
// data returned to just N bytes. Each call to Write
// updates N to reflect the new amount remaining.
// Write returns EOF when N <= 0 or when the underlying W returns EOF.
type limitedWriter struct {
	W io.Writer // underlying writer
	N int64     // max bytes remaining
}

func (l *limitedWriter) Write(p []byte) (n int, err error) {
	if l.N <= 0 {
		return 0, io.ErrShortWrite
	}
	truncated := false
	if int64(len(p)) > l.N {
		p = p[0:l.N]
		truncated = true
	}
	n, err = l.W.Write(p)
	l.N -= int64(n)
	if err == nil && truncated {
		err = io.ErrShortWrite
	}
	return
}

func (r *runtimeVM) execContainerCommon(ctx context.Context, c *Container, cmd []string, timeout int64, stdin io.Reader, stdout, stderr io.WriteCloser, tty bool, resizeChan <-chan remotecommand.TerminalSize) (exitCode int32, retErr error) {
	log.Debugf(ctx, "RuntimeVM.execContainerCommon() start")
	defer log.Debugf(ctx, "RuntimeVM.execContainerCommon() end")

	// Cancel the context before returning to ensure goroutines are stopped.
	ctx, cancel := context.WithCancel(r.ctx)
	defer cancel()

	// Generate a unique execID
	execID, err := utils.GenerateID()
	if err != nil {
		return execError, fmt.Errorf("exec container: %w", err)
	}

	// Create IO fifos
	execIO, err := cio.NewExecIO(c.ID(), r.fifoDir, tty, stdin != nil)
	if err != nil {
		return execError, errdefs.FromGRPC(err)
	}
	defer execIO.Close()

	// chan to notify that can call runtime's CloseIO API
	closeIOChan := make(chan bool)
	defer func() {
		if closeIOChan != nil {
			close(closeIOChan)
		}
	}()

	execIO.Attach(cio.AttachOptions{
		Stdin:     stdin,
		Stdout:    stdout,
		Stderr:    stderr,
		Tty:       tty,
		StdinOnce: true,
		CloseStdin: func() error {
			if closeIOChan != nil {
				<-closeIOChan
			}
			return r.closeIO(c.ID(), execID)
		},
	})

	// It's important to make a spec copy here to not overwrite the initial
	// process spec
	pSpec := *c.Spec().Process
	pSpec.Args = cmd

	any, err := typeurl.MarshalAny(&pSpec)
	if err != nil {
		return execError, errdefs.FromGRPC(err)
	}

	request := &task.ExecProcessRequest{
		ID:       c.ID(),
		ExecID:   execID,
		Stdin:    execIO.Config().Stdin,
		Stdout:   execIO.Config().Stdout,
		Stderr:   execIO.Config().Stderr,
		Terminal: execIO.Config().Terminal,
		Spec:     protobuf.FromAny(any),
	}

	// Create the "exec" process
	if _, err = r.task.Exec(r.ctx, request); err != nil {
		return execError, errdefs.FromGRPC(err)
	}

	defer func() {
		if retErr != nil {
			if err := r.remove(c.ID(), execID); err != nil {
				log.Debugf(ctx, "Unable to remove container %s: %v", c.ID(), err)
			}
		}
	}()

	// Start the process
	if err := r.start(c.ID(), execID); err != nil {
		return execError, err
	}

	// close closeIOChan to notify execIO exec has started.
	close(closeIOChan)
	closeIOChan = nil

	// Initialize terminal resizing if necessary
	if resizeChan != nil {
		utils.HandleResizing(resizeChan, func(size remotecommand.TerminalSize) {
			log.Debugf(ctx, "Got a resize event: %+v", size)

			if err := r.resizePty(c.ID(), execID, size); err != nil {
				log.Warnf(ctx, "Failed to resize terminal: %v", err)
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
		exitCode, err = r.wait(c.ID(), execID)
		if err != nil {
			execCh <- err
		}

		close(execCh)
	}()

	select {
	case err = <-execCh:
		if err != nil {
			if killErr := r.kill(c.ID(), execID, syscall.SIGKILL, false); killErr != nil {
				return execError, killErr
			}
			return execError, err
		}
	case <-timeoutCh:
		if killErr := r.kill(c.ID(), execID, syscall.SIGKILL, false); killErr != nil {
			return execError, killErr
		}
		<-execCh
		// do not make an error for timeout: report it with a specific error code
		return execTimeout, nil
	}

	if err == nil {
		// Delete the process
		if err := r.remove(c.ID(), execID); err != nil {
			log.Debugf(ctx, "Unable to remove container %s: %v", c.ID(), err)
		}
	}

	return exitCode, err
}

// UpdateContainer updates container resources
func (r *runtimeVM) UpdateContainer(ctx context.Context, c *Container, res *rspec.LinuxResources) error {
	log.Debugf(ctx, "RuntimeVM.UpdateContainer() start")
	defer log.Debugf(ctx, "RuntimeVM.UpdateContainer() end")

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
		Resources: protobuf.FromAny(any),
	}); err != nil {
		return errdefs.FromGRPC(err)
	}

	return nil
}

// StopContainer stops a container. Timeout is given in seconds.
func (r *runtimeVM) StopContainer(ctx context.Context, c *Container, timeout int64) error {
	log.Debugf(ctx, "RuntimeVM.StopContainer() start")
	defer log.Debugf(ctx, "RuntimeVM.StopContainer() end")

	if err := r.shouldBeStopped(ctx, c); err != nil {
		if errors.Is(err, ErrContainerStopped) {
			err = nil
		}
		return err
	}

	// Lock the container
	c.opLock.Lock()
	defer c.opLock.Unlock()

	// Cancel the context before returning to ensure goroutines are stopped.
	ctx, cancel := context.WithCancel(r.ctx)
	defer cancel()

	stopCh := make(chan error)
	go func() {
		// errdefs.ErrNotFound actually comes from a closed connection, which is expected
		// when stopping the container, with the agent and the VM going off. In such case.
		// let's just ignore the error.
		if _, err := r.wait(c.ID(), ""); err != nil && !errors.Is(err, errdefs.ErrNotFound) {
			stopCh <- errdefs.FromGRPC(err)
		}

		close(stopCh)
	}()

	var sig syscall.Signal

	if timeout > 0 {
		sig = c.StopSignal()
		// Send a stopping signal to the container
		if err := r.kill(c.ID(), "", sig, false); err != nil {
			return err
		}

		timeoutDuration := time.Duration(timeout) * time.Second

		err := r.waitCtrTerminate(sig, stopCh, timeoutDuration)
		if err == nil {
			c.state.Finished = time.Now()
			return nil
		}
		log.Warnf(ctx, "%v", err)
	}

	sig = syscall.SIGKILL
	// Send a SIGKILL signal to the container
	if err := r.kill(c.ID(), "", sig, false); err != nil {
		return err
	}

	if err := r.waitCtrTerminate(sig, stopCh, killContainerTimeout); err != nil {
		log.Errorf(ctx, "%v", err)
		return err
	}

	c.state.Finished = time.Now()
	return nil
}

// shouldBeStopped checks whether the container's state permits
// stopping. It determines if stopping the container makes sense
// based on its current state. A container cannot be stopped if
// it is already stopped or paused. If the container is paused,
// the function attempts to unpause it and update its status.
func (r *runtimeVM) shouldBeStopped(ctx context.Context, c *Container) error {
	switch c.State().Status {
	case ContainerStateStopped:
		return ErrContainerStopped
	case ContainerStatePaused:
		log.Warnf(ctx, "Cannot stop paused container %s", c.ID())
		if err := r.UnpauseContainer(ctx, c); err != nil {
			return fmt.Errorf("failed to stop container %s: %w", c.Name(), err)
		}
		if err := r.UpdateContainerStatus(ctx, c); err != nil {
			return fmt.Errorf("failed to update container status %s: %w", c.Name(), err)
		}
	}
	return nil
}

func (r *runtimeVM) waitCtrTerminate(sig syscall.Signal, stopCh chan error, timeout time.Duration) error {
	select {
	case err := <-stopCh:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("StopContainer with signal %v timed out after (%v)", sig, timeout)
	}
}

// DeleteContainer deletes a container.
func (r *runtimeVM) DeleteContainer(ctx context.Context, c *Container) error {
	log.Debugf(ctx, "RuntimeVM.DeleteContainer() start")
	defer log.Debugf(ctx, "RuntimeVM.DeleteContainer() end")

	// Lock the container
	c.opLock.Lock()
	defer c.opLock.Unlock()
	if c.state.OOMKilled {
		// Collect metric by container name
		metrics.Instance().MetricContainersOOMCountTotalDelete(c.Name())
	}

	return r.deleteContainer(c, false)
}

// deleteContainer performs all the operations needed to delete a container.
// force must only be used on clean-up cases.
// It does **not** Lock the container, thus it's the caller responsibility to do so, when needed.
func (r *runtimeVM) deleteContainer(c *Container, force bool) error {
	r.Lock()
	cInfo, ok := r.ctrs[c.ID()]
	r.Unlock()
	if !ok && !force {
		return errors.New("could not retrieve container information")
	}

	if err := cInfo.cio.Close(); err != nil && !force {
		return err
	}

	if err := r.remove(c.ID(), ""); err != nil && !force {
		return err
	}

	_, err := r.task.Shutdown(r.ctx, &task.ShutdownRequest{ID: c.ID()})
	if err != nil && !errors.Is(err, ttrpc.ErrClosed) && !force {
		return err
	}

	r.Lock()
	delete(r.ctrs, c.ID())
	r.Unlock()

	return nil
}

// UpdateContainerStatus refreshes the status of the container.
func (r *runtimeVM) UpdateContainerStatus(ctx context.Context, c *Container) error {
	log.Debugf(ctx, "RuntimeVM.UpdateContainerStatus() start")
	defer log.Debugf(ctx, "RuntimeVM.UpdateContainerStatus() end")

	// Lock the container
	c.opLock.Lock()
	defer c.opLock.Unlock()

	return r.updateContainerStatus(ctx, c)
}

// updateContainerStatus is a UpdateContainerStatus helper, which actually does the container's
// status refresh.
// It does **not** Lock the container, thus it's the caller responsibility to do so, when needed.
func (r *runtimeVM) updateContainerStatus(ctx context.Context, c *Container) error {
	log.Debugf(ctx, "RuntimeVM.updateContainerStatus() start")
	defer log.Debugf(ctx, "RuntimeVM.updateContainerStatus() end")

	// This can happen on restore. We need to read shim address from the bundle path.
	// And then connect to the existing gRPC server with this address.
	if r.task == nil {
		addressPath := filepath.Join(c.BundlePath(), "address")
		data, err := os.ReadFile(addressPath)
		if err != nil {
			// If the container is actually removed, this error is expected and should be ignored.
			// In this case, the container's status should be "Stopped".
			if c.state.Status == ContainerStateStopped {
				log.Debugf(ctx, "Skipping status update for: %+v", c.state)
				return nil
			}

			log.Warnf(ctx, "Failed to read shim address: %v", err)
			return errors.New("runtime not correctly setup")
		}
		address := strings.TrimSpace(string(data))
		conn, err := client.Connect(address, client.AnonDialer)
		if err != nil {
			return err
		}
		options := ttrpc.WithOnClose(func() { conn.Close() })
		cl := ttrpc.NewClient(conn, options)
		r.client = cl
		r.task = task.NewTaskClient(cl)
	}

	response, err := r.task.State(r.ctx, &task.StateRequest{
		ID: c.ID(),
	})
	if err != nil {
		if !errors.Is(err, ttrpc.ErrClosed) {
			return errdefs.FromGRPC(err)
		}
		return errdefs.ErrNotFound
	}

	if err = r.restoreContainerIO(ctx, c, response); err != nil {
		return fmt.Errorf("failed to restore container io: %w", err)
	}

	status := c.state.Status
	switch response.Status {
	case tasktypes.Status_CREATED:
		status = ContainerStateCreated
	case tasktypes.Status_RUNNING:
		status = ContainerStateRunning
	case tasktypes.Status_STOPPED:
		status = ContainerStateStopped
	case tasktypes.Status_PAUSED:
		status = ContainerStatePaused
	}

	c.state.Status = status
	c.state.Finished = response.ExitedAt.AsTime()
	exitCode := int32(response.ExitStatus)
	c.state.ExitCode = &exitCode
	c.state.Pid = int(response.Pid)

	if exitCode != 0 {
		oomFilePath := filepath.Join(c.bundlePath, "oom")
		if _, err = os.Stat(oomFilePath); err == nil {
			c.state.OOMKilled = true

			// Collect total metric
			metrics.Instance().MetricContainersOOMTotalInc()

			// Collect metric by container name
			metrics.Instance().MetricContainersOOMCountTotalInc(c.Name())
		}
	}
	return nil
}

func (r *runtimeVM) restoreContainerIO(ctx context.Context, c *Container, state *task.StateResponse) error {
	r.Lock()
	_, ok := r.ctrs[c.ID()]
	if ok {
		r.Unlock()
		return nil
	}
	r.Unlock()

	cioCfg := ctrio.Config{
		Terminal: state.Terminal,
		Stdin:    state.Stdin,
		Stdout:   state.Stdout,
		Stderr:   state.Stderr,
	}
	// The existing fifos is created by NewFIFOSetInDir. stdin, stdout, stderr should exist
	// in a same temporary directory under r.fifoDir. crio is responsible for removing these
	// files after container io is closed.
	var iofiles []string
	if cioCfg.Stdin != "" {
		iofiles = append(iofiles, cioCfg.Stdin)
	}
	if cioCfg.Stdout != "" {
		iofiles = append(iofiles, cioCfg.Stdout)
	}
	if cioCfg.Stderr != "" {
		iofiles = append(iofiles, cioCfg.Stderr)
	}
	closer := func() error {
		for _, f := range iofiles {
			if err := os.Remove(f); err != nil {
				return err
			}
		}
		// Also try to remove the parent dir if it is empty.
		for _, f := range iofiles {
			_ = os.Remove(filepath.Dir(f))
		}
		return nil
	}
	_, err := r.createContainerIO(ctx, c, cio.WithFIFOs(ctrio.NewFIFOSet(cioCfg, closer)))
	return err
}

func (r *runtimeVM) createContainerIO(ctx context.Context, c *Container, cioOpts ...cio.ContainerIOOpts) (_ *cio.ContainerIO, retErr error) {
	// Create IO fifos
	containerIO, err := cio.NewContainerIO(c.ID(), cioOpts...)
	if err != nil {
		return nil, err
	}

	defer func() {
		if retErr != nil {
			containerIO.Close()
		}
	}()

	f, err := os.OpenFile(c.LogPath(), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}

	var stdoutCh, stderrCh <-chan struct{}
	wc := cioutil.NewSerialWriteCloser(f)
	stdout, stdoutCh := cio.NewCRILogger(c.LogPath(), wc, cio.Stdout, -1)
	stderr, stderrCh := cio.NewCRILogger(c.LogPath(), wc, cio.Stderr, -1)

	go func() {
		if stdoutCh != nil {
			<-stdoutCh
		}
		if stderrCh != nil {
			<-stderrCh
		}
		log.Debugf(ctx, "Finish redirecting log file %q, closing it", c.LogPath())
		f.Close()
	}()

	containerIO.AddOutput(c.LogPath(), stdout, stderr)
	containerIO.Pipe()

	r.Lock()
	r.ctrs[c.ID()] = containerInfo{
		cio: containerIO,
	}
	r.Unlock()

	return containerIO, nil
}

// PauseContainer pauses a container.
func (r *runtimeVM) PauseContainer(ctx context.Context, c *Container) error {
	log.Debugf(ctx, "RuntimeVM.PauseContainer() start")
	defer log.Debugf(ctx, "RuntimeVM.PauseContainer() end")

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
func (r *runtimeVM) UnpauseContainer(ctx context.Context, c *Container) error {
	log.Debugf(ctx, "RuntimeVM.UnpauseContainer() start")
	defer log.Debugf(ctx, "RuntimeVM.UnpauseContainer() end")

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
func (r *runtimeVM) ContainerStats(ctx context.Context, c *Container, _ string) (*cgmgr.CgroupStats, error) {
	log.Debugf(ctx, "RuntimeVM.ContainerStats() start")
	defer log.Debugf(ctx, "RuntimeVM.ContainerStats() end")

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
		return nil, errors.New("could not retrieve container stats")
	}

	stats, err := typeurl.UnmarshalAny(resp.Stats)
	if err != nil {
		return nil, fmt.Errorf("failed to extract container metrics: %w", err)
	}

	// We can't assume the version of metrics we will get based on the host system,
	// because the guest VM may be using a different version.
	// Trying to retrieve the V1 metrics first, and if it fails, try the v2
	m, ok := stats.(*cgroupsV1.Metrics)
	if ok {
		return metricsV1ToCgroupStats(ctx, m), nil
	} else {
		m, ok := stats.(*cgroupsV2.Metrics)
		if ok {
			return metricsV2ToCgroupStats(ctx, m), nil
		}
	}
	return nil, fmt.Errorf("unknown stats type %T", stats)
}

func metricsV1ToCgroupStats(ctx context.Context, m *cgroupsV1.Metrics) *cgmgr.CgroupStats {
	var (
		memLimit        uint64
		memUsage        uint64
		workingSetBytes uint64
	)
	memUsage = m.Memory.Usage.Usage
	memLimit = cgmgr.MemLimitGivenSystem(m.Memory.Usage.Limit)
	if memUsage > m.Memory.TotalInactiveFile {
		workingSetBytes = memUsage - m.Memory.TotalInactiveFile
	} else {
		log.Debugf(ctx,
			"Unable to account working set stats: total_inactive_file (%d) > memory usage (%d)",
			m.Memory.TotalInactiveFile, memUsage,
		)
	}
	return &cgmgr.CgroupStats{
		Memory: &cgmgr.MemoryStats{
			Usage:           memUsage,
			WorkingSetBytes: workingSetBytes,
			Limit:           memLimit,
			AvailableBytes:  memLimit - workingSetBytes,
			RssBytes:        m.Memory.RSS,
			PageFaults:      m.Memory.PgFault,
			MajorPageFaults: m.Memory.PgMajFault,
			Cache:           m.Memory.Cache,
			MaxUsage:        m.Memory.Usage.Max,
			KernelUsage:     m.Memory.Kernel.Usage,
			KernelTCPUsage:  m.Memory.KernelTCP.Usage,
			SwapUsage:       m.Memory.Swap.Usage,
			SwapLimit:       m.Memory.Swap.Limit,
			// There is also m.Memory.TotalMappedFile
			// TODO: See which one is the best to use here
			FileMapped: m.Memory.MappedFile,
			Failcnt:    m.Memory.Usage.Failcnt,
		},
		CPU: &cgmgr.CPUStats{
			TotalUsageNano:          m.CPU.Usage.Total,
			PerCPUUsage:             m.CPU.Usage.PerCPU,
			ThrottledTime:           m.CPU.Throttling.ThrottledTime,
			ThrottlingActivePeriods: m.CPU.Throttling.Periods,
			ThrottledPeriods:        m.CPU.Throttling.ThrottledPeriods,
		},
		Pid: &cgmgr.PidsStats{
			Current: m.Pids.Current,
			Limit:   m.Pids.Limit,
		},
		SystemNano: time.Now().UnixNano(),
	}
}

func metricsV2ToCgroupStats(ctx context.Context, m *cgroupsV2.Metrics) *cgmgr.CgroupStats {
	var (
		memLimit        uint64
		memUsage        uint64
		workingSetBytes uint64
	)
	if m != nil {
		memUsage = m.Memory.Usage
		memLimit = cgmgr.MemLimitGivenSystem(m.Memory.UsageLimit)
		if memUsage > m.Memory.InactiveFile {
			workingSetBytes = memUsage - m.Memory.InactiveFile
		} else {
			log.Debugf(ctx,
				"Unable to account working set stats: total_inactive_file (%d) > memory usage (%d)",
				m.Memory.InactiveFile, memUsage,
			)
		}
	}
	return &cgmgr.CgroupStats{
		Memory: &cgmgr.MemoryStats{
			Usage:           memUsage,
			WorkingSetBytes: workingSetBytes,
			Limit:           memLimit,
			AvailableBytes:  memLimit - workingSetBytes,
			PageFaults:      m.Memory.Pgfault,
			MajorPageFaults: m.Memory.Pgmajfault,
			// Use Memory.Anon as Rss for cgroup v2 as in cAdvisor
			// See: https://github.com/google/cadvisor/blob/786dbcfdf5b1aae8341b47e71ab115066a9b4c06/container/libcontainer/handler.go#L809
			RssBytes: m.Memory.Anon,
			// Use Memory.File as Cache for cgroup v2 as in cAdvisor
			// See: https://github.com/google/cadvisor/blob/786dbcfdf5b1aae8341b47e71ab115066a9b4c06/container/libcontainer/handler.go#L808
			Cache:       m.Memory.File,
			KernelUsage: m.Memory.KernelStack,
			SwapUsage:   m.Memory.SwapUsage,
			SwapLimit:   m.Memory.SwapLimit,
			FileMapped:  m.Memory.FileMapped,
		},
		CPU: &cgmgr.CPUStats{
			TotalUsageNano:          m.CPU.UsageUsec * 1000,
			UsageInKernelmode:       m.CPU.SystemUsec * 1000,
			UsageInUsermode:         m.CPU.UserUsec * 1000,
			ThrottlingActivePeriods: m.CPU.NrPeriods,
			ThrottledPeriods:        m.CPU.NrThrottled,
			ThrottledTime:           m.CPU.ThrottledUsec * 1000,
		},
		Pid: &cgmgr.PidsStats{
			Current: m.Pids.Current,
			Limit:   m.Pids.Limit,
		},
		SystemNano: time.Now().UnixNano(),
	}
}

// SignalContainer sends a signal to a container process.
func (r *runtimeVM) SignalContainer(ctx context.Context, c *Container, sig syscall.Signal) error {
	log.Debugf(ctx, "RuntimeVM.SignalContainer() start")
	defer log.Debugf(ctx, "RuntimeVM.SignalContainer() end")

	// Lock the container
	c.opLock.Lock()
	defer c.opLock.Unlock()

	return r.kill(c.ID(), "", sig, true)
}

// AttachContainer attaches IO to a running container.
func (r *runtimeVM) AttachContainer(ctx context.Context, c *Container, inputStream io.Reader, outputStream, errorStream io.WriteCloser, tty bool, resizeChan <-chan remotecommand.TerminalSize) error {
	log.Debugf(ctx, "RuntimeVM.AttachContainer() start")
	defer log.Debugf(ctx, "RuntimeVM.AttachContainer() end")

	// Initialize terminal resizing
	utils.HandleResizing(resizeChan, func(size remotecommand.TerminalSize) {
		log.Debugf(ctx, "Got a resize event: %+v", size)

		if err := r.resizePty(c.ID(), "", size); err != nil {
			log.Warnf(ctx, "Failed to resize terminal: %v", err)
		}
	})

	r.Lock()
	cInfo, ok := r.ctrs[c.ID()]
	r.Unlock()
	if !ok {
		return errors.New("could not retrieve container information")
	}

	opts := cio.AttachOptions{
		Stdin:     inputStream,
		Stdout:    outputStream,
		Stderr:    errorStream,
		Tty:       tty,
		StdinOnce: c.stdinOnce,
		CloseStdin: func() error {
			return r.closeIO(c.ID(), "")
		},
	}

	cInfo.cio.Attach(opts)
	return nil
}

// PortForwardContainer forwards the specified port provides statistics of a container.
func (r *runtimeVM) PortForwardContainer(ctx context.Context, c *Container, netNsPath string, port int32, stream io.ReadWriteCloser) error {
	log.Debugf(ctx, "RuntimeVM.PortForwardContainer() start")
	defer log.Debugf(ctx, "RuntimeVM.PortForwardContainer() end")

	return nil
}

// ReopenContainerLog reopens the log file of a container.
func (r *runtimeVM) ReopenContainerLog(ctx context.Context, c *Container) error {
	log.Debugf(ctx, "RuntimeVM.ReopenContainerLog() start")
	defer log.Debugf(ctx, "RuntimeVM.ReopenContainerLog() end")

	return nil
}

func (r *runtimeVM) start(ctrID, execID string) error {
	if _, err := r.task.Start(r.ctx, &task.StartRequest{
		ID:     ctrID,
		ExecID: execID,
	}); err != nil {
		return errdefs.FromGRPC(err)
	}

	return nil
}

func (r *runtimeVM) wait(ctrID, execID string) (int32, error) {
	resp, err := r.task.Wait(r.ctx, &task.WaitRequest{
		ID:     ctrID,
		ExecID: execID,
	})
	if err != nil {
		if !errors.Is(err, ttrpc.ErrClosed) {
			return -1, errdefs.FromGRPC(err)
		}
		return -1, errdefs.ErrNotFound
	}

	return int32(resp.ExitStatus), nil
}

func (r *runtimeVM) kill(ctrID, execID string, signal syscall.Signal, all bool) error {
	if _, err := r.task.Kill(r.ctx, &task.KillRequest{
		ID:     ctrID,
		ExecID: execID,
		Signal: uint32(signal),
		All:    all,
	}); err != nil {
		return errdefs.FromGRPC(err)
	}

	return nil
}

func (r *runtimeVM) remove(ctrID, execID string) error {
	if _, err := r.task.Delete(r.ctx, &task.DeleteRequest{
		ID:     ctrID,
		ExecID: execID,
	}); err != nil && !errors.Is(err, ttrpc.ErrClosed) {
		return errdefs.FromGRPC(err)
	}

	return nil
}

func (r *runtimeVM) resizePty(ctrID, execID string, size remotecommand.TerminalSize) error {
	_, err := r.task.ResizePty(r.ctx, &task.ResizePtyRequest{
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

func (r *runtimeVM) closeIO(ctrID, execID string) error {
	_, err := r.task.CloseIO(r.ctx, &task.CloseIORequest{
		ID:     ctrID,
		ExecID: execID,
		Stdin:  true,
	})
	if err != nil {
		return errdefs.FromGRPC(err)
	}

	return nil
}

// CheckpointContainer not implemented for runtimeVM
func (r *runtimeVM) CheckpointContainer(ctx context.Context, c *Container, specgen *rspec.Spec, leaveRunning bool) error {
	log.Debugf(ctx, "RuntimeVM.CheckpointContainer() start")
	defer log.Debugf(ctx, "RuntimeVM.CheckpointContainer() end")

	return errors.New("checkpointing not implemented for runtimeVM")
}

// RestoreContainer not implemented for runtimeVM
func (r *runtimeVM) RestoreContainer(ctx context.Context, c *Container, cgroupParent, mountLabel string) error {
	log.Debugf(ctx, "RuntimeVM.RestoreContainer() start")
	defer log.Debugf(ctx, "RuntimeVM.RestoreContainer() end")

	return errors.New("restoring not implemented for runtimeVM")
}

func EncodeKataVirtualVolumeToBase64(ctx context.Context, volume *katavolume.KataVirtualVolume) (string, error) {
	validKataVirtualVolumeJSON, err := json.Marshal(volume)
	if err != nil {
		return "", fmt.Errorf("marshal KataVirtualVolume object; error=%w", err)
	}
	log.Infof(ctx, "Encode kata volume %v", validKataVirtualVolumeJSON)
	option := base64.StdEncoding.EncodeToString(validKataVirtualVolumeJSON)
	return option, nil
}
