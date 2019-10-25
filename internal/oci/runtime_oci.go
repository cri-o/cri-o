package oci

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cri-o/cri-o/internal/lib/config"
	"github.com/cri-o/cri-o/internal/pkg/findprocess"
	"github.com/cri-o/cri-o/utils"
	"github.com/docker/docker/pkg/pools"
	"github.com/fsnotify/fsnotify"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	kwait "k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/remotecommand"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
	utilexec "k8s.io/utils/exec"
)

const (
	// RuntimeTypeOCI is the type representing the RuntimeOCI implementation.
	RuntimeTypeOCI = "oci"

	// Command line flag used to specify the run root directory
	rootFlag = "--root"
)

// runtimeOCI is the Runtime interface implementation relying on conmon to
// interact with the container runtime.
type runtimeOCI struct {
	*Runtime

	path string
	root string
}

// newRuntimeOCI creates a new runtimeOCI instance
func newRuntimeOCI(r *Runtime, handler *config.RuntimeHandler) RuntimeImpl {
	runRoot := config.DefaultRuntimeRoot
	if handler.RuntimeRoot != "" {
		runRoot = handler.RuntimeRoot
	}
	return &runtimeOCI{
		Runtime: r,
		path:    handler.RuntimePath,
		root:    runRoot,
	}
}

// syncInfo is used to return data from monitor process to daemon
type syncInfo struct {
	Pid     int    `json:"pid"`
	Message string `json:"message,omitempty"`
}

// exitCodeInfo is used to return the monitored process exit code to the daemon
type exitCodeInfo struct {
	ExitCode int32  `json:"exit_code"`
	Message  string `json:"message,omitempty"`
}

// CreateContainer creates a container.
func (r *runtimeOCI) CreateContainer(c *Container, cgroupParent string) (err error) {
	var stderrBuf bytes.Buffer
	parentPipe, childPipe, err := newPipe()
	childStartPipe, parentStartPipe, err := newPipe()
	if err != nil {
		return fmt.Errorf("error creating socket pair: %v", err)
	}
	defer parentPipe.Close()
	defer parentStartPipe.Close()

	var args []string
	if r.config.CgroupManager == SystemdCgroupsManager {
		args = append(args, "-s")
	}
	if r.config.CgroupManager == CgroupfsCgroupsManager {
		args = append(args, "--syslog")
	}

	args = append(args,
		"-c", c.id,
		"-n", c.name,
		"-u", c.id,
		"-r", r.path,
		"-b", c.bundlePath,
		"-p", filepath.Join(c.bundlePath, "pidfile"),
		"-l", c.logPath,
		"--exit-dir", r.config.ContainerExitsDir,
		"--socket-dir-path", r.config.ContainerAttachSocketDir,
		"--log-level", logrus.GetLevel().String(),
		"--runtime-arg", fmt.Sprintf("%s=%s", rootFlag, r.root))
	if r.config.LogSizeMax >= 0 {
		args = append(args, "--log-size-max", fmt.Sprintf("%v", r.config.LogSizeMax))
	}
	if r.config.LogToJournald {
		args = append(args, "--log-path", "journald:")
	}
	if r.config.NoPivot {
		args = append(args, "--no-pivot")
	}
	if c.terminal {
		args = append(args, "-t")
	} else if c.stdin {
		if !c.stdinOnce {
			args = append(args, "--leave-stdin-open")
		}
		args = append(args, "-i")
	}
	logrus.WithFields(logrus.Fields{
		"args": args,
	}).Debugf("running conmon: %s", r.config.Conmon)

	cmd := exec.Command(r.config.Conmon, args...) // nolint: gosec
	cmd.Dir = c.bundlePath
	cmd.SysProcAttr = sysProcAttrPlatform()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if c.terminal {
		cmd.Stderr = &stderrBuf
	}
	cmd.ExtraFiles = append(cmd.ExtraFiles, childPipe, childStartPipe)
	// 0, 1 and 2 are stdin, stdout and stderr
	cmd.Env = r.config.ConmonEnv
	cmd.Env = append(cmd.Env,
		fmt.Sprintf("_OCI_SYNCPIPE=%d", 3),
		fmt.Sprintf("_OCI_STARTPIPE=%d", 4))
	if v, found := os.LookupEnv("XDG_RUNTIME_DIR"); found {
		cmd.Env = append(cmd.Env, fmt.Sprintf("XDG_RUNTIME_DIR=%s", v))
	}

	err = cmd.Start()
	if err != nil {
		childPipe.Close()
		return err
	}

	// We don't need childPipe on the parent side
	childPipe.Close()
	childStartPipe.Close()

	// Platform specific container setup
	r.createContainerPlatform(c, cgroupParent, cmd.Process.Pid)

	/* We set the cgroup, now the child can start creating children */
	someData := []byte{0}
	_, err = parentStartPipe.Write(someData)
	if err != nil {
		if waitErr := cmd.Wait(); waitErr != nil {
			return errors.Wrap(err, waitErr.Error())
		}
		return err
	}

	/* Wait for initial setup and fork, and reap child */
	err = cmd.Wait()
	if err != nil {
		return err
	}

	// We will delete all container resources if creation fails
	defer func() {
		if err != nil {
			if err := r.DeleteContainer(c); err != nil {
				logrus.Warnf("unable to delete container %s: %v", c.ID(), err)
			}
		}
	}()

	// Wait to get container pid from conmon
	type syncStruct struct {
		si  *syncInfo
		err error
	}
	ch := make(chan syncStruct)
	go func() {
		var si *syncInfo
		if err = json.NewDecoder(parentPipe).Decode(&si); err != nil {
			ch <- syncStruct{err: err}
			return
		}
		ch <- syncStruct{si: si}
	}()

	select {
	case ss := <-ch:
		if ss.err != nil {
			return fmt.Errorf("error reading container (probably exited) json message: %v", ss.err)
		}
		logrus.Debugf("Received container pid: %d", ss.si.Pid)
		if ss.si.Pid == -1 {
			if ss.si.Message != "" {
				logrus.Errorf("Container creation error: %s", ss.si.Message)
				return fmt.Errorf("container create failed: %s", ss.si.Message)
			}
			logrus.Errorf("Container creation failed")
			return fmt.Errorf("container create failed")
		}
	case <-time.After(ContainerCreateTimeout):
		logrus.Errorf("Container creation timeout (%v)", ContainerCreateTimeout)
		return fmt.Errorf("create container timeout")
	}
	return nil
}

// StartContainer starts a container.
func (r *runtimeOCI) StartContainer(c *Container) error {
	c.opLock.Lock()
	defer c.opLock.Unlock()

	if err := utils.ExecCmdWithStdStreams(os.Stdin, os.Stdout, os.Stderr,
		r.path, rootFlag, r.root, "start", c.id); err != nil {
		return err
	}
	c.state.Started = time.Now()
	return nil
}

func prepareExec() (pidFile, parentPipe, childPipe *os.File, err error) {
	parentPipe, childPipe, err = os.Pipe()
	if err != nil {
		return nil, nil, nil, err
	}

	pidFile, err = ioutil.TempFile("", "pidfile")
	if err != nil {
		parentPipe.Close()
		childPipe.Close()
		return nil, nil, nil, err
	}

	return
}

func parseLog(log []byte) (stdout, stderr []byte) {
	// Split the log on newlines, which is what separates entries.
	lines := bytes.SplitAfter(log, []byte{'\n'})
	for _, line := range lines {
		// Ignore empty lines.
		if len(line) == 0 {
			continue
		}

		// The format of log lines is "DATE pipe LogTag REST".
		parts := bytes.SplitN(line, []byte{' '}, 4)
		if len(parts) < 4 {
			// Ignore the line if it's formatted incorrectly, but complain
			// about it so it can be debugged.
			logrus.Warnf("hit invalid log format: %q", string(line))
			continue
		}

		pipe := string(parts[1])
		content := parts[3]

		linetype := string(parts[2])
		if linetype == "P" {
			contentLen := len(content)
			if contentLen > 0 && content[contentLen-1] == '\n' {
				content = content[:contentLen-1]
			}
		}

		switch pipe {
		case "stdout":
			stdout = append(stdout, content...)
		case "stderr":
			stderr = append(stderr, content...)
		default:
			// Complain about unknown pipes.
			logrus.Warnf("hit invalid log format [unknown pipe %s]: %q", pipe, string(line))
			continue
		}
	}

	return stdout, stderr
}

// ExecContainer prepares a streaming endpoint to execute a command in the container.
func (r *runtimeOCI) ExecContainer(c *Container, cmd []string, stdin io.Reader, stdout, stderr io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) error {
	processFile, err := prepareProcessExec(c, cmd, tty)
	if err != nil {
		return err
	}
	defer os.RemoveAll(processFile.Name())

	args := []string{rootFlag, r.root, "exec"}
	args = append(args, "--process", processFile.Name(), c.ID())
	execCmd := exec.Command(r.path, args...) // nolint: gosec
	if v, found := os.LookupEnv("XDG_RUNTIME_DIR"); found {
		execCmd.Env = append(execCmd.Env, fmt.Sprintf("XDG_RUNTIME_DIR=%s", v))
	}
	var cmdErr, copyError error
	if tty {
		cmdErr = ttyCmd(execCmd, stdin, stdout, resize)
	} else {
		if stdin != nil {
			// Use an os.Pipe here as it returns true *os.File objects.
			// This way, if you run 'kubectl exec <pod> -i bash' (no tty) and type 'exit',
			// the call below to execCmd.Run() can unblock because its Stdin is the read half
			// of the pipe.
			r, w, err := os.Pipe()
			if err != nil {
				return err
			}
			go func() { _, copyError = pools.Copy(w, stdin) }()

			execCmd.Stdin = r
		}
		if stdout != nil {
			execCmd.Stdout = stdout
		}
		if stderr != nil {
			execCmd.Stderr = stderr
		}

		cmdErr = execCmd.Run()
	}

	if copyError != nil {
		return copyError
	}
	if exitErr, ok := cmdErr.(*exec.ExitError); ok {
		return &utilexec.ExitErrorWrapper{ExitError: exitErr}
	}
	return cmdErr
}

// ExecSyncContainer execs a command in a container and returns it's stdout, stderr and return code.
func (r *runtimeOCI) ExecSyncContainer(c *Container, command []string, timeout int64) (resp *ExecSyncResponse, err error) {
	pidFile, parentPipe, childPipe, err := prepareExec()
	if err != nil {
		return nil, &ExecSyncError{
			ExitCode: -1,
			Err:      err,
		}
	}
	defer parentPipe.Close()
	defer func() {
		if e := os.Remove(pidFile.Name()); e != nil {
			logrus.Warnf("could not remove temporary PID file %s", pidFile.Name())
		}
	}()

	logFile, err := ioutil.TempFile("", "crio-log-"+c.id)
	if err != nil {
		return nil, &ExecSyncError{
			ExitCode: -1,
			Err:      err,
		}
	}
	logPath := logFile.Name()
	defer func() {
		logFile.Close()
		os.RemoveAll(logPath)
	}()

	var args []string
	args = append(args,
		"-c", c.id,
		"-n", c.name,
		"-r", r.path,
		"-p", pidFile.Name(),
		"-e")
	if c.terminal {
		args = append(args, "-t")
	}
	if timeout > 0 {
		args = append(args, "-T", fmt.Sprintf("%d", timeout))
	}
	args = append(args,
		"-l", logPath,
		"--socket-dir-path", r.config.ContainerAttachSocketDir,
		"--log-level", logrus.GetLevel().String())

	processFile, err := prepareProcessExec(c, command, c.terminal)
	if err != nil {
		return nil, &ExecSyncError{
			ExitCode: -1,
			Err:      err,
		}
	}
	defer os.RemoveAll(processFile.Name())

	args = append(args,
		"--exec-process-spec", processFile.Name(),
		"--runtime-arg", fmt.Sprintf("%s=%s", rootFlag, r.root))

	cmd := exec.Command(r.config.Conmon, args...) // nolint: gosec

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	cmd.ExtraFiles = append(cmd.ExtraFiles, childPipe)
	// 0, 1 and 2 are stdin, stdout and stderr
	cmd.Env = r.config.ConmonEnv
	cmd.Env = append(cmd.Env, fmt.Sprintf("_OCI_SYNCPIPE=%d", 3))
	if v, found := os.LookupEnv("XDG_RUNTIME_DIR"); found {
		cmd.Env = append(cmd.Env, fmt.Sprintf("XDG_RUNTIME_DIR=%s", v))
	}

	err = cmd.Start()
	if err != nil {
		childPipe.Close()
		return nil, &ExecSyncError{
			Stdout:   stdoutBuf,
			Stderr:   stderrBuf,
			ExitCode: -1,
			Err:      err,
		}
	}

	// We don't need childPipe on the parent side
	childPipe.Close()

	err = cmd.Wait()
	if err != nil {
		return nil, &ExecSyncError{
			Stdout:   stdoutBuf,
			Stderr:   stderrBuf,
			ExitCode: getExitCode(err),
			Err:      err,
		}
	}

	var ec *exitCodeInfo
	if err := json.NewDecoder(parentPipe).Decode(&ec); err != nil {
		return nil, &ExecSyncError{
			Stdout:   stdoutBuf,
			Stderr:   stderrBuf,
			ExitCode: -1,
			Err:      err,
		}
	}

	logrus.Debugf("Received container exit code: %v, message: %s", ec.ExitCode, ec.Message)

	if ec.ExitCode == -1 {
		return nil, &ExecSyncError{
			Stdout:   stdoutBuf,
			Stderr:   stderrBuf,
			ExitCode: -1,
			Err:      fmt.Errorf(ec.Message),
		}
	}

	// The actual logged output is not the same as stdoutBuf and stderrBuf,
	// which are used for getting error information. For the actual
	// ExecSyncResponse we have to read the logfile.
	// XXX: Currently runC dups the same console over both stdout and stderr,
	//      so we can't differentiate between the two.

	logBytes, err := ioutil.ReadFile(logPath)
	if err != nil {
		return nil, &ExecSyncError{
			Stdout:   stdoutBuf,
			Stderr:   stderrBuf,
			ExitCode: -1,
			Err:      err,
		}
	}

	// We have to parse the log output into {stdout, stderr} buffers.
	stdoutBytes, stderrBytes := parseLog(logBytes)
	return &ExecSyncResponse{
		Stdout:   stdoutBytes,
		Stderr:   stderrBytes,
		ExitCode: ec.ExitCode,
	}, nil
}

// UpdateContainer updates container resources
func (r *runtimeOCI) UpdateContainer(c *Container, res *rspec.LinuxResources) error {
	cmd := exec.Command(r.path, rootFlag, r.root, "update", "--resources", "-", c.id) // nolint: gosec
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if v, found := os.LookupEnv("XDG_RUNTIME_DIR"); found {
		cmd.Env = append(cmd.Env, fmt.Sprintf("XDG_RUNTIME_DIR=%s", v))
	}
	jsonResources, err := json.Marshal(res)
	if err != nil {
		return err
	}
	cmd.Stdin = bytes.NewReader(jsonResources)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("updating resources for container %q failed: %v %v (%v)", c.id, stderr.String(), stdout.String(), err)
	}
	return nil
}

func waitContainerStop(ctx context.Context, c *Container, timeout time.Duration, ignoreKill bool) error {
	done := make(chan struct{})
	// we could potentially re-use "done" channel to exit the loop on timeout,
	// but we use another channel "chControl" so that we never panic
	// attempting to close an already-closed "done" channel.  The panic
	// would occur in the "default" select case below if we'd closed the
	// "done" channel (instead of the "chControl" channel) in the timeout
	// select case.
	chControl := make(chan struct{})
	go func() {
		for {
			select {
			case <-chControl:
				return
			default:
				process, err := findprocess.FindProcess(c.state.Pid)
				if err != nil {
					if err != findprocess.ErrNotFound {
						logrus.Warnf("failed to find process %d for container %q: %v", c.state.Pid, c.id, err)
					}
					close(done)
					return
				}
				err = process.Release()
				if err != nil {
					logrus.Warnf("failed to release process %d for container %q: %v", c.state.Pid, c.id, err)
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		close(chControl)
		return ctx.Err()
	case <-time.After(timeout):
		close(chControl)
		if ignoreKill {
			return fmt.Errorf("failed to wait process, timeout reached after %.0f seconds",
				timeout.Seconds())
		}
		err := kill(c.state.Pid)
		if err != nil {
			return fmt.Errorf("failed to kill process: %v", err)
		}
	}

	c.state.Finished = time.Now()
	return nil
}

// StopContainer stops a container. Timeout is given in seconds.
func (r *runtimeOCI) StopContainer(ctx context.Context, c *Container, timeout int64) error {
	c.opLock.Lock()
	defer c.opLock.Unlock()

	// Check if the process is around before sending a signal
	process, err := findprocess.FindProcess(c.state.Pid)
	if err == findprocess.ErrNotFound {
		c.state.Finished = time.Now()
		return nil
	}
	if err != nil {
		logrus.Warnf("failed to find process %d for container %q: %v", c.state.Pid, c.id, err)
	} else {
		err = process.Release()
		if err != nil {
			logrus.Warnf("failed to release process %d for container %q: %v", c.state.Pid, c.id, err)
		}
	}

	if timeout > 0 {
		if err := utils.ExecCmdWithStdStreams(os.Stdin, os.Stdout, os.Stderr,
			r.path, rootFlag, r.root, "kill", c.id, c.GetStopSignal()); err != nil {
			if err := checkProcessGone(c); err != nil {
				return fmt.Errorf("failed to stop container %q: %v", c.id, err)
			}
		}
		err = waitContainerStop(ctx, c, time.Duration(timeout)*time.Second, true)
		if err == nil {
			return nil
		}
		logrus.Warnf("Stop container %q timed out: %v", c.id, err)
	}

	if err := utils.ExecCmdWithStdStreams(os.Stdin, os.Stdout, os.Stderr,
		r.path, rootFlag, r.root, "kill", c.id, "KILL"); err != nil {
		if err := checkProcessGone(c); err != nil {
			return fmt.Errorf("failed to stop container %q: %v", c.id, err)
		}
	}

	return waitContainerStop(ctx, c, killContainerTimeout, false)
}

func checkProcessGone(c *Container) error {
	process, perr := findprocess.FindProcess(c.state.Pid)
	if perr == findprocess.ErrNotFound {
		c.state.Finished = time.Now()
		return nil
	}
	if perr == nil {
		err := process.Release()
		if err != nil {
			logrus.Warnf("failed to release process %d for container %q: %v", c.state.Pid, c.id, err)
		}
	}
	return fmt.Errorf("failed to find process: %v", perr)
}

// DeleteContainer deletes a container.
func (r *runtimeOCI) DeleteContainer(c *Container) error {
	c.opLock.Lock()
	defer c.opLock.Unlock()

	_, err := utils.ExecCmd(r.path, rootFlag, r.root, "delete", "--force", c.id)
	return err
}

// UpdateContainerStatus refreshes the status of the container.
func (r *runtimeOCI) UpdateContainerStatus(c *Container) error {
	c.opLock.Lock()
	defer c.opLock.Unlock()

	cmd := exec.Command(r.path, rootFlag, r.root, "state", c.id) // nolint: gosec
	if v, found := os.LookupEnv("XDG_RUNTIME_DIR"); found {
		cmd.Env = append(cmd.Env, fmt.Sprintf("XDG_RUNTIME_DIR=%s", v))
	}
	out, err := cmd.Output()
	if err != nil {
		// there are many code paths that could lead to have a bad state in the
		// underlying runtime.
		// On any error like a container went away or we rebooted and containers
		// went away we do not error out stopping kubernetes to recover.
		// We always populate the fields below so kube can restart/reschedule
		// containers failing.
		c.state.Status = ContainerStateStopped
		c.state.Finished = time.Now()
		c.state.ExitCode = 255
		return nil
	}
	if err := json.NewDecoder(bytes.NewBuffer(out)).Decode(&c.state); err != nil {
		return fmt.Errorf("failed to decode container status for %s: %s", c.id, err)
	}

	if c.state.Status == ContainerStateStopped {
		exitFilePath := filepath.Join(r.config.ContainerExitsDir, c.id)
		var fi os.FileInfo
		err = kwait.ExponentialBackoff(
			kwait.Backoff{
				Duration: 500 * time.Millisecond,
				Factor:   1.2,
				Steps:    6,
			},
			func() (bool, error) {
				var err error
				fi, err = os.Stat(exitFilePath)
				if err != nil {
					// wait longer
					return false, nil
				}
				return true, nil
			})
		if err != nil {
			logrus.Warnf("failed to find container exit file for %v: %v", c.id, err)
			c.state.ExitCode = -1
		} else {
			c.state.Finished, err = getFinishedTime(fi)
			if err != nil {
				return fmt.Errorf("failed to get finished time: %v", err)
			}
			statusCodeStr, err := ioutil.ReadFile(exitFilePath)
			if err != nil {
				return fmt.Errorf("failed to read exit file: %v", err)
			}
			statusCode, err := strconv.Atoi(string(statusCodeStr))
			if err != nil {
				return fmt.Errorf("status code conversion failed: %v", err)
			}
			c.state.ExitCode = int32(statusCode)
		}

		oomFilePath := filepath.Join(c.bundlePath, "oom")
		if _, err = os.Stat(oomFilePath); err == nil {
			c.state.OOMKilled = true
		}
	}

	return nil
}

// PauseContainer pauses a container.
func (r *runtimeOCI) PauseContainer(c *Container) error {
	c.opLock.Lock()
	defer c.opLock.Unlock()

	_, err := utils.ExecCmd(r.path, rootFlag, r.root, "pause", c.id)
	return err
}

// UnpauseContainer unpauses a container.
func (r *runtimeOCI) UnpauseContainer(c *Container) error {
	c.opLock.Lock()
	defer c.opLock.Unlock()

	_, err := utils.ExecCmd(r.path, rootFlag, r.root, "resume", c.id)
	return err
}

func (r *runtimeOCI) WaitContainerStateStopped(ctx context.Context, c *Container) error {
	return nil
}

// ContainerStats provides statistics of a container.
func (r *runtimeOCI) ContainerStats(c *Container) (*ContainerStats, error) {
	c.opLock.Lock()
	defer c.opLock.Unlock()

	return r.containerStats(c)
}

// SignalContainer sends a signal to a container process.
func (r *runtimeOCI) SignalContainer(c *Container, sig syscall.Signal) error {
	c.opLock.Lock()
	defer c.opLock.Unlock()

	signalString, err := findStringInSignalMap(sig)
	if err != nil {
		return err
	}

	return utils.ExecCmdWithStdStreams(os.Stdin, os.Stdout, os.Stderr, r.path,
		rootFlag, r.root, "kill", c.ID(), signalString)
}

// AttachContainer attaches IO to a running container.
func (r *runtimeOCI) AttachContainer(c *Container, inputStream io.Reader, outputStream, errorStream io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) error {
	controlPath := filepath.Join(c.BundlePath(), "ctl")
	controlFile, err := os.OpenFile(controlPath, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("failed to open container ctl file: %v", err)
	}
	defer controlFile.Close()

	kubecontainer.HandleResizing(resize, func(size remotecommand.TerminalSize) {
		logrus.Debugf("Got a resize event: %+v", size)
		_, err := fmt.Fprintf(controlFile, "%d %d %d\n", 1, size.Height, size.Width)
		if err != nil {
			logrus.Debugf("Failed to write to control file to resize terminal: %v", err)
		}
	})

	attachSocketPath := filepath.Join(r.config.ContainerAttachSocketDir, c.ID(), "attach")
	conn, err := net.DialUnix("unixpacket", nil, &net.UnixAddr{Name: attachSocketPath, Net: "unixpacket"})
	if err != nil {
		return fmt.Errorf("failed to connect to container %s attach socket: %v", c.ID(), err)
	}
	defer conn.Close()

	receiveStdout := make(chan error)
	if outputStream != nil || errorStream != nil {
		go func() {
			receiveStdout <- redirectResponseToOutputStreams(outputStream, errorStream, conn)
		}()
	}

	stdinDone := make(chan error)
	go func() {
		var err error
		if inputStream != nil {
			_, err = utils.CopyDetachable(conn, inputStream, nil)
			if closeErr := conn.CloseWrite(); closeErr != nil {
				stdinDone <- closeErr
			}
		}
		stdinDone <- err
	}()

	select {
	case err := <-receiveStdout:
		return err
	case err := <-stdinDone:
		if !c.StdinOnce() && !tty {
			return nil
		}
		if _, ok := err.(utils.DetachError); ok {
			return nil
		}
		if outputStream != nil || errorStream != nil {
			return <-receiveStdout
		}
	}

	return nil
}

// PortForwardContainer forwards the specified port provides statistics of a container.
func (r *runtimeOCI) PortForwardContainer(c *Container, port int32, stream io.ReadWriter) error {
	containerPid := c.State().Pid
	socatPath, lookupErr := exec.LookPath("socat")
	if lookupErr != nil {
		return fmt.Errorf("unable to do port forwarding: socat not found")
	}

	args := []string{"-t", fmt.Sprintf("%d", containerPid), "-n", socatPath, "-", fmt.Sprintf("TCP4:localhost:%d", port)}

	nsenterPath, lookupErr := exec.LookPath("nsenter")
	if lookupErr != nil {
		return fmt.Errorf("unable to do port forwarding: nsenter not found")
	}

	commandString := fmt.Sprintf("%s %s", nsenterPath, strings.Join(args, " "))
	logrus.Debugf("executing port forwarding command: %s", commandString)

	command := exec.Command(nsenterPath, args...)
	if v, found := os.LookupEnv("XDG_RUNTIME_DIR"); found {
		command.Env = append(command.Env, fmt.Sprintf("XDG_RUNTIME_DIR=%s", v))
	}
	command.Stdout = stream

	stderr := new(bytes.Buffer)
	command.Stderr = stderr

	// If we use Stdin, command.Run() won't return until the goroutine that's copying
	// from stream finishes. Unfortunately, if you have a client like telnet connected
	// via port forwarding, as long as the user's telnet client is connected to the user's
	// local listener that port forwarding sets up, the telnet session never exits. This
	// means that even if socat has finished running, command.Run() won't ever return
	// (because the client still has the connection and stream open).
	//
	// The work around is to use StdinPipe(), as Wait() (called by Run()) closes the pipe
	// when the command (socat) exits.
	inPipe, err := command.StdinPipe()
	if err != nil {
		return fmt.Errorf("unable to do port forwarding: error creating stdin pipe: %v", err)
	}
	var copyError error
	go func() {
		_, copyError = pools.Copy(inPipe, stream)
		inPipe.Close()
	}()

	runErr := command.Run()

	if copyError != nil {
		return copyError
	}

	if runErr != nil {
		return fmt.Errorf("%v: %s", runErr, stderr.String())
	}

	return nil
}

// ReopenContainerLog reopens the log file of a container.
func (r *runtimeOCI) ReopenContainerLog(c *Container) error {
	controlPath := filepath.Join(c.BundlePath(), "ctl")
	controlFile, err := os.OpenFile(controlPath, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("failed to open container ctl file: %v", err)
	}
	defer controlFile.Close()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create new watch: %v", err)
	}
	defer watcher.Close()

	done := make(chan struct{})
	errorCh := make(chan error)
	go func() {
		for {
			select {
			case event := <-watcher.Events:
				logrus.Debugf("event: %v", event)
				if event.Op&fsnotify.Create == fsnotify.Create || event.Op&fsnotify.Write == fsnotify.Write {
					logrus.Debugf("file created %s", event.Name)
					if event.Name == c.LogPath() {
						logrus.Debugf("expected log file created")
						close(done)
						return
					}
				}
			case err := <-watcher.Errors:
				errorCh <- fmt.Errorf("watch error for container log reopen %v: %v", c.ID(), err)
				return
			}
		}
	}()
	cLogDir := filepath.Dir(c.LogPath())
	if err := watcher.Add(cLogDir); err != nil {
		logrus.Errorf("watcher.Add(%q) failed: %s", cLogDir, err)
		close(done)
	}

	if _, err = fmt.Fprintf(controlFile, "%d %d %d\n", 2, 0, 0); err != nil {
		logrus.Debugf("Failed to write to control file to reopen log file: %v", err)
	}

	select {
	case err := <-errorCh:
		return err
	case <-done:
		break
	}

	return nil
}

// prepareProcessExec returns the path of the process.json used in runc exec -p
// caller is responsible to close the returned *os.File if needed.
func prepareProcessExec(c *Container, cmd []string, tty bool) (*os.File, error) {
	f, err := ioutil.TempFile("", "exec-process-")
	if err != nil {
		return nil, err
	}

	pspec := c.Spec().Process
	pspec.Args = cmd
	// We need to default this to false else it will inherit terminal as true
	// from the container.
	pspec.Terminal = false
	if tty {
		pspec.Terminal = true
	}
	processJSON, err := json.Marshal(pspec)
	if err != nil {
		return nil, err
	}

	if err := ioutil.WriteFile(f.Name(), processJSON, 0644); err != nil {
		return nil, err
	}
	return f, nil
}
