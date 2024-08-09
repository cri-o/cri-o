package oci

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	criu "github.com/checkpoint-restore/go-criu/v7/utils"
	"github.com/containers/common/pkg/crutils"
	conmonconfig "github.com/containers/conmon/runner/config"
	"github.com/containers/storage/pkg/pools"
	"github.com/fsnotify/fsnotify"
	json "github.com/json-iterator/go"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"golang.org/x/sys/unix"
	kwait "k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/remotecommand"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	kclock "k8s.io/utils/clock"
	utilexec "k8s.io/utils/exec"

	"github.com/cri-o/cri-o/internal/config/cgmgr"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/pkg/config"
	"github.com/cri-o/cri-o/server/metrics"
	"github.com/cri-o/cri-o/utils"
	"github.com/cri-o/cri-o/utils/cmdrunner"
)

const (
	// RuntimeTypeOCI is the type representing the RuntimeOCI implementation.
	RuntimeTypeOCI = "oci"

	// Command line flag used to specify the run root directory.
	rootFlag = "--root"

	// Configuration for the stop loop exponential backoff manager.
	stopInitialBackoff = 20 * time.Millisecond
	stopMaximumBackoff = 2 * time.Minute
	stopResetBackoff   = 5 * time.Minute
	stopBackoffFactor  = 2.0
	stopBackoffJitter  = 1.25

	// When to start the blocked process reminder and
	// how frequently the reminder should be shown.
	stopProcessBlockedInterval = stopMaximumBackoff / 2

	// Used to delay periodic process liveness check. Part of the
	// container stop loop where a goroutine wakes up on a regular
	// basis to check whether a given PID (process) continues to
	// run. This allows to short-circuit stop logic if the process
	// has already been terminated.
	stopProcessWatchSleep = 100 * time.Millisecond
)

// runtimeOCI is the Runtime interface implementation relying on conmon to
// interact with the container runtime.
type runtimeOCI struct {
	*Runtime

	root    string
	handler *config.RuntimeHandler
}

// newRuntimeOCI creates a new runtimeOCI instance.
func newRuntimeOCI(r *Runtime, handler *config.RuntimeHandler) RuntimeImpl {
	runRoot := config.DefaultRuntimeRoot
	if handler.RuntimeRoot != "" {
		runRoot = handler.RuntimeRoot
	}

	return &runtimeOCI{
		Runtime: r,
		root:    runRoot,
		handler: handler,
	}
}

// syncInfo is used to return data from monitor process to daemon.
type syncInfo struct {
	Pid     int    `json:"pid"`
	Message string `json:"message,omitempty"`
}

// exitCodeInfo is used to return the monitored process exit code to the daemon.
type exitCodeInfo struct {
	ExitCode int32  `json:"exit_code"`
	Message  string `json:"message,omitempty"`
}

// CreateContainer creates a container.
func (r *runtimeOCI) CreateContainer(ctx context.Context, c *Container, cgroupParent string, restore bool) (retErr error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	if c.Spoofed() {
		return nil
	}

	var stderrBuf bytes.Buffer
	parentPipe, childPipe, err := newPipe()
	if err != nil {
		return fmt.Errorf("error creating socket pair: %w", err)
	}
	childStartPipe, parentStartPipe, err := newPipe()
	if err != nil {
		return fmt.Errorf("error creating socket pair: %w", err)
	}
	defer parentPipe.Close()
	defer parentStartPipe.Close()

	args := []string{
		"-b", c.bundlePath,
		"-c", c.ID(),
		"--exit-dir", r.config.ContainerExitsDir,
		"-l", c.logPath,
		"--log-level", logrus.GetLevel().String(),
		"-n", c.name,
		"-P", c.conmonPidFilePath(),
		"-p", filepath.Join(c.bundlePath, "pidfile"),
		"--persist-dir", c.dir,
		"-r", c.RuntimePathForPlatform(r),
		"--runtime-arg", fmt.Sprintf("%s=%s", rootFlag, r.root),
		"--socket-dir-path", r.config.ContainerAttachSocketDir,
		"--syslog",
		"-u", c.ID(),
	}

	if r.config.CgroupManager().IsSystemd() {
		args = append(args, "-s")
	}
	if r.config.LogSizeMax >= 0 {
		args = append(args, "--log-size-max", strconv.FormatInt(r.config.LogSizeMax, 10))
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
	if restore {
		log.Debugf(ctx, "Restore is true %v", restore)
		args = append(args, "--restore", c.CheckpointPath())
		if c.Spec().Process.SelinuxLabel != "" {
			args = append(
				args,
				"--runtime-opt",
				"--lsm-profile=selinux:"+c.Spec().Process.SelinuxLabel,
			)
		}
		if c.Spec().Linux.MountLabel != "" {
			args = append(
				args,
				"--runtime-opt",
				"--lsm-mount-context="+c.Spec().Linux.MountLabel,
			)
		}
	}

	log.WithFields(ctx, logrus.Fields{
		"args": args,
	}).Debugf("running conmon: %s", r.handler.MonitorPath)

	cmd := cmdrunner.Command(r.handler.MonitorPath, args...) //nolint: gosec
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
	cmd.Env = r.handler.MonitorEnv
	cmd.Env = append(cmd.Env,
		fmt.Sprintf("_OCI_SYNCPIPE=%d", 3),
		fmt.Sprintf("_OCI_STARTPIPE=%d", 4))
	if v, found := os.LookupEnv("XDG_RUNTIME_DIR"); found {
		cmd.Env = append(cmd.Env, "XDG_RUNTIME_DIR="+v)
	}
	if restore {
		// The CRIU binary is usually in /usr/sbin/criu
		if v, found := os.LookupEnv("PATH"); found {
			cmd.Env = append(cmd.Env, "PATH="+v)
		}
	}

	err = cmd.Start()
	if err != nil {
		childPipe.Close()
		childStartPipe.Close()
		return err
	}

	// We don't need childPipe on the parent side
	childPipe.Close()
	childStartPipe.Close()

	// Create new scope to reduce cleanup code.
	if err := func() (retErr error) {
		defer func() {
			if retErr != nil {
				// We need to always kill and wait on this process.
				// Failing to do so will cause us to leak a zombie.
				killErr := cmd.Process.Kill()
				waitErr := cmd.Wait()
				if killErr != nil {
					retErr = fmt.Errorf("failed to kill %w after failing with: %w", killErr, retErr)
				}
				// Per https://pkg.go.dev/os#ProcessState.ExitCode, the exit code is -1 when the process died because
				// of a signal. We expect this in this case, as we've just killed it with a signal. Don't append the
				// error in this case to reduce noise.
				var exitErr *exec.ExitError
				if !errors.As(waitErr, &exitErr) || exitErr.ExitCode() != -1 {
					retErr = fmt.Errorf("failed to wait %w after failing with: %w", waitErr, retErr)
				}
			}
		}()
		// Platform specific container setup
		if !c.Spoofed() {
			if err := r.createContainerPlatform(c, cgroupParent, cmd.Process.Pid); err != nil {
				return err
			}
		}

		/* We set the cgroup, now the child can start creating children */
		someData := []byte{0}
		_, err = parentStartPipe.Write(someData)
		return err
	}(); err != nil {
		return err
	}

	defer func() {
		if retErr != nil {
			if err := os.Remove(c.logPath); err != nil {
				log.Warnf(ctx, "Failed to remove log path %s after failing to create container: %v", c.logPath, err)
			}
		}
	}()
	/* Wait for initial setup and fork, and reap child */
	err = cmd.Wait()
	if err != nil {
		return err
	}

	// We will delete all container resources if creation fails
	defer func() {
		if retErr != nil {
			if err := r.DeleteContainer(ctx, c); err != nil {
				log.Warnf(ctx, "Unable to delete container %s: %v", c.ID(), err)
			}
		}
	}()

	// Wait to get container pid from conmon
	type syncStruct struct {
		si  *syncInfo
		err error
	}
	ch := make(chan syncStruct, 1)
	go func() {
		defer close(ch)
		var si *syncInfo
		if err = json.NewDecoder(parentPipe).Decode(&si); err != nil {
			ch <- syncStruct{err: err}
			return
		}
		ch <- syncStruct{si: si}
	}()

	var pid int
	select {
	case ss := <-ch:
		if ss.err != nil {
			return fmt.Errorf("error reading container (probably exited) json message: %w", ss.err)
		}
		log.Debugf(ctx, "Received container pid: %d", ss.si.Pid)
		pid = ss.si.Pid
		if ss.si.Pid == -1 {
			if ss.si.Message != "" {
				if restore {
					log.Errorf(ctx, "Container restore error: %s", ss.si.Message)
					return fmt.Errorf("container restore failed: %s", ss.si.Message)
				}
				log.Errorf(ctx, "Container creation error: %s", ss.si.Message)
				return fmt.Errorf("container create failed: %s", ss.si.Message)
			}
			log.Errorf(ctx, "Container creation failed")
			return errors.New("container create failed")
		}
	case <-time.After(ContainerCreateTimeout):
		log.Errorf(ctx, "Container creation timeout (%v)", ContainerCreateTimeout)
		return errors.New("create container timeout")
	}

	// Now we know the container has started, save the pid to verify against future calls.
	if err := c.state.SetInitPid(pid); err != nil {
		return err
	}

	return nil
}

// StartContainer starts a container.
func (r *runtimeOCI) StartContainer(ctx context.Context, c *Container) error {
	_, span := log.StartSpan(ctx)
	defer span.End()
	c.opLock.Lock()
	defer c.opLock.Unlock()

	if c.Spoofed() {
		return nil
	}

	if _, err := r.runtimeCmd("start", c.ID()); err != nil {
		return err
	}
	c.state.Started = time.Now()
	return nil
}

func prepareExec() (pidFileName string, parentPipe, childPipe *os.File, _ error) {
	var err error
	parentPipe, childPipe, err = os.Pipe()
	if err != nil {
		return "", nil, nil, err
	}

	pidFile, err := os.CreateTemp("", "pidfile")
	if err != nil {
		parentPipe.Close()
		childPipe.Close()
		return "", nil, nil, err
	}
	pidFile.Close()
	pidFileName = pidFile.Name()

	return pidFileName, parentPipe, childPipe, nil
}

func parseLog(ctx context.Context, l []byte) (stdout, stderr []byte) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	// Split the log on newlines, which is what separates entries.
	lines := bytes.SplitAfter(l, []byte{'\n'})
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
			log.Warnf(ctx, "Hit invalid log format: %q", string(line))
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
			log.Warnf(ctx, "Hit invalid log format [unknown pipe %s]: %q", pipe, string(line))
			continue
		}
	}

	return stdout, stderr
}

// ExecContainer prepares a streaming endpoint to execute a command in the container.
func (r *runtimeOCI) ExecContainer(ctx context.Context, c *Container, cmd []string, stdin io.Reader, stdout, stderr io.WriteCloser, tty bool, resizeChan <-chan remotecommand.TerminalSize) error {
	_, span := log.StartSpan(ctx)
	defer span.End()

	if c.Spoofed() {
		return nil
	}

	processFile, err := prepareProcessExec(c, cmd, tty)
	if err != nil {
		return err
	}
	defer os.RemoveAll(processFile)

	args := r.defaultRuntimeArgs()
	args = append(args, "exec", "--process", processFile, c.ID())
	execCmd := cmdrunner.CommandContext(ctx, c.RuntimePathForPlatform(r), args...) //nolint: gosec
	if v, found := os.LookupEnv("XDG_RUNTIME_DIR"); found {
		execCmd.Env = append(execCmd.Env, "XDG_RUNTIME_DIR="+v)
	}
	var cmdErr, copyError error
	if tty {
		execCmd.WaitDelay = 30 * time.Second
		cmdErr = ttyCmd(execCmd, stdin, stdout, resizeChan, c)
	} else {
		var r, w *os.File
		if stdin != nil {
			// Use an os.Pipe here as it returns true *os.File objects.
			// This way, if you run 'kubectl exec <pod> -i bash' (no tty) and type 'exit',
			// the call below to execCmd.Run() can unblock because its Stdin is the read half
			// of the pipe.
			r, w, err = os.Pipe()
			if err != nil {
				return err
			}

			u := c.Spec().Process.User
			// Change the owner for the pipe to the user in the container
			if err := unix.Fchown(int(r.Fd()), int(u.UID), int(u.GID)); err != nil {
				return err
			}

			execCmd.Stdin = r
			go func() {
				_, copyError = pools.Copy(w, stdin)
				w.Close()
			}()
		}

		if stdout != nil {
			execCmd.Stdout = stdout
		}

		if stderr != nil {
			execCmd.Stderr = stderr
		}

		if err := execCmd.Start(); err != nil {
			return err
		}

		pid := execCmd.Process.Pid
		if err := c.AddExecPID(pid, true); err != nil {
			return err
		}
		defer c.DeleteExecPID(pid)

		// The read side of the pipe should be closed after the container process has been started.
		if r != nil {
			if err := r.Close(); err != nil {
				if waitErr := execCmd.Wait(); waitErr != nil {
					return fmt.Errorf("%w: %w", waitErr, err)
				}
				return err
			}
		}

		cmdErr = execCmd.Wait()
	}

	if copyError != nil {
		return copyError
	}
	var exitErr *exec.ExitError
	if errors.As(cmdErr, &exitErr) {
		return &utilexec.ExitErrorWrapper{ExitError: exitErr}
	}
	return cmdErr
}

// ExecSyncContainer execs a command in a container and returns it's stdout, stderr and return code.
func (r *runtimeOCI) ExecSyncContainer(ctx context.Context, c *Container, command []string, timeout int64) (*types.ExecSyncResponse, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	if c.Spoofed() {
		return nil, nil
	}

	pidFile, parentPipe, childPipe, err := prepareExec()
	if err != nil {
		return nil, &ExecSyncError{
			ExitCode: -1,
			Err:      err,
		}
	}

	childStartPipe, parentStartPipe, err := newPipe()
	if err != nil {
		return nil, &ExecSyncError{
			ExitCode: -1,
			Err:      err,
		}
	}

	defer parentPipe.Close()
	defer parentStartPipe.Close()
	defer func() {
		if e := os.Remove(pidFile); e != nil {
			log.Warnf(ctx, "Could not remove temporary PID file %s", pidFile)
		}
	}()

	logFile, err := os.CreateTemp("", "crio-log-"+c.ID())
	if err != nil {
		return nil, &ExecSyncError{
			ExitCode: -1,
			Err:      err,
		}
	}
	logFile.Close()

	logPath := logFile.Name()
	defer func() {
		os.RemoveAll(logPath)
	}()

	args := []string{
		"-c", c.ID(),
		"-n", c.name,
		"-r", c.RuntimePathForPlatform(r),
		"-p", pidFile,
		"-e",
		"-l", logPath,
		"--socket-dir-path", r.config.ContainerAttachSocketDir,
		"--log-level", logrus.GetLevel().String(),
	}

	if r.config.ConmonSupportsSync() {
		args = append(args, "--sync")
	}
	if r.config.ConmonSupportsLogGlobalSizeMax() {
		args = append(args, "--log-global-size-max", strconv.Itoa(maxExecSyncSize))
	}
	if c.terminal {
		args = append(args, "-t")
	}
	if timeout > 0 {
		args = append(args, "-T", strconv.FormatInt(timeout, 10))
	}
	if r.config.CgroupManager().IsSystemd() {
		args = append(args, "-s")
	}

	processFile, err := prepareProcessExec(c, command, c.terminal)
	if err != nil {
		return nil, &ExecSyncError{
			ExitCode: -1,
			Err:      err,
		}
	}
	defer os.RemoveAll(processFile)

	args = append(args,
		"--exec-process-spec", processFile,
		"--runtime-arg", fmt.Sprintf("%s=%s", rootFlag, r.root))

	var cmd *exec.Cmd

	if r.handler.MonitorExecCgroup == config.MonitorExecCgroupDefault || r.config.InfraCtrCPUSet == "" { //nolint: gocritic
		cmd = cmdrunner.Command(r.handler.MonitorPath, args...) //nolint: gosec
	} else if r.handler.MonitorExecCgroup == config.MonitorExecCgroupContainer {
		cmd = exec.Command(r.handler.MonitorPath, args...) //nolint: gosec
	} else {
		msg := "Unsupported monitor_exec_cgroup value: " + r.handler.MonitorExecCgroup
		return &types.ExecSyncResponse{
			Stderr:   []byte(msg),
			ExitCode: -1,
		}, nil
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	cmd.ExtraFiles = append(cmd.ExtraFiles, childPipe, childStartPipe)
	// 0, 1 and 2 are stdin, stdout and stderr
	cmd.Env = r.handler.MonitorEnv
	cmd.Env = append(cmd.Env,
		fmt.Sprintf("_OCI_SYNCPIPE=%d", 3),
		fmt.Sprintf("_OCI_STARTPIPE=%d", 4))

	if v, found := os.LookupEnv("XDG_RUNTIME_DIR"); found {
		cmd.Env = append(cmd.Env, "XDG_RUNTIME_DIR="+v)
	}

	err = cmd.Start()
	if err != nil {
		childPipe.Close()
		childStartPipe.Close()

		return nil, &ExecSyncError{
			Stdout:   stdoutBuf,
			Stderr:   stderrBuf,
			ExitCode: -1,
			Err:      err,
		}
	}

	// We don't need childPipe on the parent side
	childPipe.Close()
	childStartPipe.Close()

	// Create new scope to reduce cleanup code.
	if err := func() (retErr error) {
		defer func() {
			if retErr != nil {
				// We need to always kill and wait on this process.
				// Failing to do so will cause us to leak a zombie.
				killErr := cmd.Process.Kill()
				waitErr := cmd.Wait()
				if killErr != nil {
					retErr = fmt.Errorf("failed to kill %w after failing with: %w", killErr, retErr)
				}
				// Per https://pkg.go.dev/os#ProcessState.ExitCode, the exit code is -1 when the process died because
				// of a signal. We expect this in this case, as we've just killed it with a signal. Don't append the
				// error in this case to reduce noise.
				var exitErr *exec.ExitError
				if !errors.As(waitErr, &exitErr) || exitErr.ExitCode() != -1 {
					retErr = fmt.Errorf("failed to wait %w after failing with: %w", waitErr, retErr)
				}
			}
		}()

		// A neat trick we can do is register the exec PID before we send info down the start pipe.
		// Doing so guarantees we can short circuit the exec process if the container is stopping already.
		if err := c.AddExecPID(cmd.Process.Pid, false); err != nil {
			return err
		}

		if r.handler.MonitorExecCgroup == config.MonitorExecCgroupContainer && r.config.InfraCtrCPUSet != "" {
			// Update the exec's cgroup
			containerPid, _, err := c.pid()
			if err != nil {
				return err
			}

			err = cgmgr.MoveProcessToContainerCgroup(containerPid, cmd.Process.Pid)
			if err != nil {
				return err
			}
		}

		// Unblock children
		someData := []byte{0}
		_, err = parentStartPipe.Write(someData)
		if err != nil {
			return err
		}

		return nil
	}(); err != nil {
		return nil, &ExecSyncError{
			Stdout:   stdoutBuf,
			Stderr:   stderrBuf,
			ExitCode: -1,
			Err:      err,
		}
	}

	// defer in case the Pid is changed after Wait()
	pid := cmd.Process.Pid

	// first, wait till the command is done
	waitErr := cmd.Wait()

	c.DeleteExecPID(pid)

	// regardless of what is in waitErr
	// we should attempt to decode the output of the parent pipe
	// this allows us to catch TimedOutMessage, which will cause waitErr to not be nil
	var ec *exitCodeInfo
	decodeErr := json.NewDecoder(parentPipe).Decode(&ec)
	if decodeErr == nil {
		log.Debugf(ctx, "Received container exit code: %v, message: %s", ec.ExitCode, ec.Message)

		// When we timeout the command in conmon then we should return
		// an ExecSyncResponse with a non-zero exit code because
		// the prober code in the kubelet checks for it. If we return
		// a custom error, then the probes transition into Unknown status
		// and the container isn't restarted as expected.
		if ec.ExitCode == -1 && ec.Message == conmonconfig.TimedOutMessage {
			return &types.ExecSyncResponse{
				Stderr:   []byte(conmonconfig.TimedOutMessage),
				ExitCode: -1,
			}, nil
		}
	}

	if waitErr != nil {
		// if we aren't a ExitError, some I/O problems probably occurred
		var exitErr *exec.ExitError
		if !errors.As(waitErr, &exitErr) {
			return nil, &ExecSyncError{
				Stdout:   stdoutBuf,
				Stderr:   stderrBuf,
				ExitCode: -1,
				Err:      waitErr,
			}
		}
	}

	if decodeErr != nil {
		return nil, &ExecSyncError{
			Stdout:   stdoutBuf,
			Stderr:   stderrBuf,
			ExitCode: -1,
			Err:      decodeErr,
		}
	}

	if ec.ExitCode == -1 {
		return nil, &ExecSyncError{
			Stdout:   stdoutBuf,
			Stderr:   stderrBuf,
			ExitCode: -1,
			Err:      errors.New(ec.Message),
		}
	}

	// The actual logged output is not the same as stdoutBuf and stderrBuf,
	// which are used for getting error information. For the actual
	// ExecSyncResponse we have to read the logfile.
	// XXX: Currently runC dups the same console over both stdout and stderr,
	//      so we can't differentiate between the two.
	logBytes, err := TruncateAndReadFile(ctx, logPath, maxExecSyncSize)
	if err != nil {
		return nil, &ExecSyncError{
			Stdout:   stdoutBuf,
			Stderr:   stderrBuf,
			ExitCode: -1,
			Err:      err,
		}
	}

	// We have to parse the log output into {stdout, stderr} buffers.
	stdoutBytes, stderrBytes := parseLog(ctx, logBytes)
	return &types.ExecSyncResponse{
		Stdout:   stdoutBytes,
		Stderr:   stderrBytes,
		ExitCode: ec.ExitCode,
	}, nil
}

func TruncateAndReadFile(ctx context.Context, path string, size int64) ([]byte, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > size {
		log.Errorf(ctx, "Exec sync output in file %s has size %d which is longer than expected size of %d", path, info.Size(), size)
		if err := os.Truncate(path, size); err != nil {
			return nil, err
		}
	}
	return os.ReadFile(path)
}

// UpdateContainer updates container resources.
func (r *runtimeOCI) UpdateContainer(ctx context.Context, c *Container, res *rspec.LinuxResources) error {
	_, span := log.StartSpan(ctx)
	defer span.End()

	c.opLock.Lock()
	defer c.opLock.Unlock()

	if c.Spoofed() {
		return nil
	}

	cmd := cmdrunner.Command(c.RuntimePathForPlatform(r), rootFlag, r.root, "update", "--resources", "-", c.ID()) //nolint: gosec
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if v, found := os.LookupEnv("XDG_RUNTIME_DIR"); found {
		cmd.Env = append(cmd.Env, "XDG_RUNTIME_DIR="+v)
	}
	jsonResources, err := json.Marshal(res)
	if err != nil {
		return err
	}
	cmd.Stdin = bytes.NewReader(jsonResources)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("updating resources for container %q failed: %v %v: %w", c.ID(), stderr.String(), stdout.String(), err)
	}
	return nil
}

// StopContainer stops a container. Timeout is given in seconds.
func (r *runtimeOCI) StopContainer(ctx context.Context, c *Container, timeout int64) (retErr error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	if c.Spoofed() {
		c.state.Status = ContainerStateStopped
		c.state.Finished = time.Now()
		return nil
	}

	// The initial container process either doesn't exist, or isn't ours.
	if err := c.Living(); err != nil {
		c.state.Finished = time.Now()
		return nil
	}

	if c.SetAsStopping() {
		// The API is due to be deprecated. However, the replacement is completely broken, see:
		//   https://github.com/kubernetes/kubernetes/issues/118638
		go r.StopLoopForContainer(c,
			kwait.NewExponentialBackoffManager( //nolint:staticcheck // Ignore deprecated function warning.
				stopInitialBackoff,
				stopMaximumBackoff,
				stopResetBackoff,
				stopBackoffFactor,
				stopBackoffJitter,
				&kclock.RealClock{},
			),
		)
	}

	c.WaitOnStopTimeout(ctx, timeout)
	return nil
}

func (r *runtimeOCI) StopLoopForContainer(c *Container, bm kwait.BackoffManager) {
	ctx := context.Background()
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	startTime := time.Now()

	// Allow for SIGINT to correctly interrupt the stop loop, especially
	// when CRI-O is run directly in the foreground in the terminal.
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)

	c.opLock.Lock()
	defer func() {
		// Kill the exec PIDs after the main container to avoid pod lifecycle regressions:
		// Ref: https://github.com/kubernetes/kubernetes/issues/124743
		c.KillExecPIDs()
		c.state.Finished = time.Now()
		c.opLock.Unlock()
		c.SetAsDoneStopping()
	}()
	if c.state.Status == ContainerStatePaused {
		if _, err := r.runtimeCmd("resume", c.ID()); err != nil {
			log.Errorf(ctx, "Failed to unpause container %s: %v", c.Name(), err)
		}
	}

	// Begin the actual kill.
	if _, err := r.runtimeCmd("kill", c.ID(), c.GetStopSignal()); err != nil {
		if err := c.Living(); err != nil {
			// The initial container process either doesn't exist, or isn't ours.
			// Set state accordingly.
			c.state.Finished = time.Now()
			return
		}
	}

	done := make(chan struct{})
	go func() {
		statusCheckTicker := time.NewTicker(stopProcessWatchSleep)
		defer statusCheckTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-statusCheckTicker.C:
				// Periodically check if the container is still running.
				// This avoids busy-waiting and reduces resource usage while
				// ensuring timely detection of container termination.
				if err := c.Living(); err != nil {
					// The initial container process either doesn't exist, or isn't ours.
					if !errors.Is(err, ErrNotFound) {
						log.Warnf(ctx, "Failed to find process for container %s: %v", c.ID(), err)
					}
					close(done)
					return
				}
			}
		}
	}()

	// Operate in terms of targetTime, so that we can pause in the middle of the operation
	// to catch a new timeout (and possibly ignore that new timeout if it's not correct to
	// take a new one).
	targetTime := time.Now().AddDate(+1, 0, 0) // A year from this one.

	blockedTimer := time.AfterFunc(stopProcessBlockedInterval, func() {
		if state, err := c.ProcessState(); err == nil && state == "D" {
			log.Errorf(ctx,
				"Detected process (%d) blocked in uninterruptible sleep for more than %d seconds for container %s",
				c.state.InitPid, int(time.Since(startTime)/time.Second), c.ID(),
			)
		}
	})
	defer blockedTimer.Stop()

	// Do not start the stuck process reminder immediately.
	blockedTimer.Stop()

	for {
		select {
		case newTimeout := <-c.stopTimeoutChan:
			// If a new timeout comes in, interrupt the old one, and start a new one.
			newTargetTime := time.Now().Add(time.Duration(newTimeout) * time.Second)

			// But, only if it's an earlier one.
			if newTargetTime.Before(targetTime) {
				targetTime = newTargetTime
			}

		case <-time.After(time.Until(targetTime)):
			log.Warnf(ctx, "Stopping container %s with stop signal timed out. Killing...", c.ID())
			goto killContainer

		case <-done:
			stop()
			return
		case <-ctx.Done():
			return
		}
	}
killContainer:
	// We cannot use ExponentialBackoff() here as its stop conditions are not flexible enough.
	kwait.BackoffUntil(func() {
		if _, err := r.runtimeCmd("kill", c.ID(), "KILL"); err != nil {
			if !errors.Is(err, ErrNotFound) {
				log.Errorf(ctx, "Killing container %v failed: %v", c.ID(), err)
			}
			log.Debugf(ctx, "Error while killing container %s: %v", c.ID(), err)
		}

		if err := c.Living(); err != nil {
			stop()
			return
		}
		// Reschedule the timer so that the periodic reminder can continue.
		blockedTimer.Reset(stopProcessBlockedInterval)
	}, bm, true, ctx.Done())
}

// DeleteContainer deletes a container.
func (r *runtimeOCI) DeleteContainer(ctx context.Context, c *Container) error {
	_, span := log.StartSpan(ctx)
	defer span.End()
	c.opLock.Lock()
	defer c.opLock.Unlock()

	if c.Spoofed() {
		return nil
	}

	if c.state.OOMKilled {
		// Collect metric by container name
		metrics.Instance().MetricContainersOOMCountTotalDelete(c.Name())
	}

	_, err := r.runtimeCmd("delete", "--force", c.ID())
	return err
}

func updateContainerStatusFromExitFile(c *Container) error {
	exitFilePath := c.exitFilePath()
	fi, err := os.Stat(exitFilePath)
	if err != nil {
		return fmt.Errorf("failed to find container exit file for %s: %w", c.ID(), err)
	}
	c.state.Finished, err = getFinishedTime(fi)
	if err != nil {
		return fmt.Errorf("failed to get finished time: %w", err)
	}
	statusCodeStr, err := os.ReadFile(exitFilePath)
	if err != nil {
		return fmt.Errorf("failed to read exit file: %w", err)
	}
	statusCode, err := strconv.ParseInt(string(statusCodeStr), 10, 32)
	if err != nil {
		return fmt.Errorf("status code conversion failed: %w", err)
	}
	c.state.ExitCode = utils.Int32Ptr(int32(statusCode))
	return nil
}

// UpdateContainerStatus refreshes the status of the container.
func (r *runtimeOCI) UpdateContainerStatus(ctx context.Context, c *Container) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	c.opLock.Lock()
	defer c.opLock.Unlock()

	if c.Spoofed() {
		return nil
	}

	if c.state.ExitCode != nil && !c.state.Finished.IsZero() {
		log.Debugf(ctx, "Skipping status update for: %+v", c.state)
		return nil
	}

	stateCmd := func() (*ContainerState, bool, error) {
		out, err := r.runtimeCmd("state", c.ID())
		if err != nil {
			// there are many code paths that could lead to have a bad state in the
			// underlying runtime.
			// On any error like a container went away or we rebooted and containers
			// went away we do not error out stopping kubernetes to recover.
			// We always populate the fields below so kube can restart/reschedule
			// containers failing.
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				log.Errorf(ctx, "Failed to update container state for %s: stdout: %s, stderr: %s", c.ID(), out, string(exitErr.Stderr))
			} else {
				log.Errorf(ctx, "Failed to update container state for %s: %v", c.ID(), err)
			}
			c.state.Status = ContainerStateStopped
			if err := updateContainerStatusFromExitFile(c); err != nil {
				c.state.Finished = time.Now()
				c.state.ExitCode = utils.Int32Ptr(255)
			}
			return nil, true, nil
		}
		state := *c.state
		if err := json.NewDecoder(strings.NewReader(out)).Decode(&state); err != nil {
			return &state, false, fmt.Errorf("failed to decode container status for %s: %w", c.ID(), err)
		}
		return &state, false, nil
	}
	state, canReturn, err := stateCmd()
	if err != nil {
		return err
	}
	if canReturn {
		return nil
	}

	if state.Status != ContainerStateStopped {
		*c.state = *state
		return nil
	}
	// release the lock before waiting
	c.opLock.Unlock()
	exitFilePath := c.exitFilePath()
	err = kwait.ExponentialBackoff(
		kwait.Backoff{
			Duration: 500 * time.Millisecond,
			Factor:   1.2,
			Steps:    6,
		},
		func() (bool, error) {
			_, err := os.Stat(exitFilePath)
			if err != nil {
				// wait longer
				return false, nil
			}
			return true, nil
		})
	c.opLock.Lock()
	// run command again
	state, _, err2 := stateCmd()
	if err2 != nil {
		return err2
	}
	if state == nil {
		return errors.New("state command returned nil")
	}
	*c.state = *state
	if err != nil {
		log.Warnf(ctx, "Failed to find container exit file for %v: %v", c.ID(), err)
	} else {
		if err := updateContainerStatusFromExitFile(c); err != nil {
			return err
		}
		log.Debugf(ctx, "Found exit code for %s: %d", c.ID(), *c.state.ExitCode)
	}

	oomFilePath := filepath.Join(c.bundlePath, "oom")
	if _, err = os.Stat(oomFilePath); err == nil {
		c.state.OOMKilled = true

		// Collect total metric
		metrics.Instance().MetricContainersOOMTotalInc()

		// Collect metric by container name
		metrics.Instance().MetricContainersOOMCountTotalInc(c.Name())
	}
	// If this container had a node level PID namespace, then any children processes will be leaked to init.
	// Eventually, the processes will get cleaned up when the pod cgroup is cleaned by the kubelet,
	// but this situation is atypical and should be avoided.
	if c.nodeLevelPIDNamespace() {
		return r.signalContainer(c, syscall.SIGKILL, true)
	}
	return nil
}

// PauseContainer pauses a container.
func (r *runtimeOCI) PauseContainer(ctx context.Context, c *Container) error {
	c.opLock.Lock()
	defer c.opLock.Unlock()

	if c.Spoofed() {
		return nil
	}

	_, err := r.runtimeCmd("pause", c.ID())
	return err
}

// UnpauseContainer unpauses a container.
func (r *runtimeOCI) UnpauseContainer(ctx context.Context, c *Container) error {
	c.opLock.Lock()
	defer c.opLock.Unlock()

	if c.Spoofed() {
		return nil
	}

	_, err := r.runtimeCmd("resume", c.ID())
	return err
}

// ContainerStats provides statistics of a container.
func (r *runtimeOCI) ContainerStats(ctx context.Context, c *Container, cgroup string) (*cgmgr.CgroupStats, error) {
	_, span := log.StartSpan(ctx)
	defer span.End()
	c.opLock.Lock()
	defer c.opLock.Unlock()
	return r.config.CgroupManager().ContainerCgroupStats(cgroup, c.ID())
}

// SignalContainer sends a signal to a container process.
func (r *runtimeOCI) SignalContainer(ctx context.Context, c *Container, sig syscall.Signal) error {
	_, span := log.StartSpan(ctx)
	defer span.End()
	c.opLock.Lock()
	defer c.opLock.Unlock()

	if c.Spoofed() {
		return nil
	}

	if unix.SignalName(sig) == "" {
		return fmt.Errorf("unable to find signal %s", sig.String())
	}

	return r.signalContainer(c, sig, false)
}

func (r *runtimeOCI) signalContainer(c *Container, sig syscall.Signal, all bool) error {
	args := []string{
		"kill",
	}
	if all {
		args = append(args, "-a")
	}
	args = append(args, c.ID(), strconv.Itoa(int(sig)))
	_, err := r.runtimeCmd(args...)
	return err
}

// AttachContainer attaches IO to a running container.
func (r *runtimeOCI) AttachContainer(ctx context.Context, c *Container, inputStream io.Reader, outputStream, errorStream io.WriteCloser, tty bool, resizeChan <-chan remotecommand.TerminalSize) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	if c.Spoofed() {
		return nil
	}

	controlPath := filepath.Join(c.BundlePath(), "ctl")
	controlFile, err := os.OpenFile(controlPath, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("failed to open container ctl file: %w", err)
	}
	defer controlFile.Close()

	utils.HandleResizing(resizeChan, func(size remotecommand.TerminalSize) {
		log.Debugf(ctx, "Got a resize event: %+v", size)
		_, err := fmt.Fprintf(controlFile, "%d %d %d\n", 1, size.Height, size.Width)
		if err != nil {
			log.Debugf(ctx, "Failed to write to control file to resize terminal: %v", err)
		}
	})

	attachSocketPath := filepath.Join(r.config.ContainerAttachSocketDir, c.ID(), "attach")
	conn, err := net.DialUnix("unixpacket", nil, &net.UnixAddr{Name: attachSocketPath, Net: "unixpacket"})
	if err != nil {
		return fmt.Errorf("failed to connect to container %s attach socket: %w", c.ID(), err)
	}
	defer conn.Close()

	receiveStdout := make(chan error)
	go func() {
		receiveStdout <- redirectResponseToOutputStreams(outputStream, errorStream, conn)
		close(receiveStdout)
	}()

	stdinDone := make(chan error)
	go func() {
		var err, closeErr error
		if inputStream != nil {
			_, err = utils.CopyDetachable(conn, inputStream, nil)
			closeErr = conn.CloseWrite()
		}
		switch {
		case err != nil:
			stdinDone <- err
		case closeErr != nil:
			stdinDone <- closeErr
		default:
			// neither CopyDetachable nor CloseWrite returned error
			stdinDone <- nil
		}
		close(stdinDone)
	}()

	select {
	case err := <-receiveStdout:
		return err
	case err := <-stdinDone:
		// This particular case is for when we get a non-tty attach
		// with --leave-stdin-open=true. We want to return as soon
		// as we receive EOF from the client. However, we should do
		// this only when stdin is enabled. If there is no stdin
		// enabled then we wait for output as usual.
		if c.stdin && !c.StdinOnce() && !tty {
			return nil
		}
		var detachErr utils.DetachError
		if errors.As(err, &detachErr) {
			return nil
		}
		return <-receiveStdout
	}
}

// ReopenContainerLog reopens the log file of a container.
func (r *runtimeOCI) ReopenContainerLog(ctx context.Context, c *Container) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	if c.Spoofed() {
		return nil
	}

	controlPath := filepath.Join(c.BundlePath(), "ctl")
	controlFile, err := os.OpenFile(controlPath, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("failed to open container ctl file: %w", err)
	}
	defer controlFile.Close()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create new watch: %w", err)
	}
	defer watcher.Close()

	done := make(chan struct{})
	doneClosed := false
	errorCh := make(chan error)
	go func() {
		for {
			select {
			case event := <-watcher.Events:
				log.Debugf(ctx, "Event: %v", event)
				if event.Op&fsnotify.Create == fsnotify.Create || event.Op&fsnotify.Write == fsnotify.Write {
					log.Debugf(ctx, "File created %s", event.Name)
					if event.Name == c.LogPath() {
						log.Debugf(ctx, "Expected log file created")
						done <- struct{}{}
						return
					}
				}
			case err := <-watcher.Errors:
				errorCh <- fmt.Errorf("watch error for container log reopen %v: %w", c.ID(), err)
				close(errorCh)
				return
			}
		}
	}()
	cLogDir := filepath.Dir(c.LogPath())
	if err := watcher.Add(cLogDir); err != nil {
		log.Errorf(ctx, "Watcher.Add(%q) failed: %s", cLogDir, err)
		close(done)
		doneClosed = true
	}

	if _, err = fmt.Fprintf(controlFile, "%d %d %d\n", 2, 0, 0); err != nil {
		log.Debugf(ctx, "Failed to write to control file to reopen log file: %v", err)
	}

	select {
	case err := <-errorCh:
		if !doneClosed {
			close(done)
		}
		return err
	case <-done:
		if !doneClosed {
			close(done)
		}
		break
	}

	return nil
}

// prepareProcessExec returns the path of the process.json used in runc exec -p
// caller is responsible for removing the returned file, if prepareProcessExec succeeds.
func prepareProcessExec(c *Container, cmd []string, tty bool) (processFile string, retErr error) {
	f, err := os.CreateTemp("", "exec-process-")
	if err != nil {
		return "", err
	}
	f.Close()
	processFile = f.Name()
	defer func(processFile string) {
		if retErr != nil {
			os.RemoveAll(processFile)
		}
	}(processFile)

	// It's important to make a spec copy here to not overwrite the initial
	// process spec
	pspec := *c.Spec().Process
	pspec.Args = cmd
	// We need to default this to false else it will inherit terminal as true
	// from the container.
	pspec.Terminal = false
	if tty {
		pspec.Terminal = true
	}
	processJSON, err := json.Marshal(pspec)
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(processFile, processJSON, 0o644); err != nil {
		return "", err
	}
	return processFile, nil
}

// ReadConmonPidFile attempts to read conmon's pid from its pid file
// This function makes no verification that this file should exist
// it is up to the caller to verify that this container has a conmon.
func ReadConmonPidFile(c *Container) (int, error) {
	contents, err := os.ReadFile(c.conmonPidFilePath())
	if err != nil {
		return -1, err
	}
	// Convert it to an int
	conmonPID, err := strconv.Atoi(string(contents))
	if err != nil {
		return -1, err
	}
	return conmonPID, nil
}

func (c *Container) conmonPidFilePath() string {
	return filepath.Join(c.bundlePath, "conmon-pidfile")
}

// runtimeCmd executes a command with args and returns its output as a string along
// with an error, if any.
func (r *runtimeOCI) runtimeCmd(args ...string) (string, error) {
	runtimeArgs := append(r.defaultRuntimeArgs(), args...)
	cmd := cmdrunner.Command(r.handler.RuntimePath, runtimeArgs...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if v, found := os.LookupEnv("XDG_RUNTIME_DIR"); found {
		cmd.Env = append(cmd.Env, "XDG_RUNTIME_DIR="+v)
	}

	err := cmd.Run()
	if err != nil {
		stdErrStr := stderr.String()
		switch {
		// crun, for most of the commands.
		case strings.Contains(stdErrStr, "no such process"):
			fallthrough //nolint:gocritic
		// runc, for most of the commands.
		case strings.Contains(stdErrStr, "container not running"):
			fallthrough //nolint:gocritic
		// runc, on a rare occasion.
		case strings.Contains(stdErrStr, "invalid process"):
			err = ErrNotFound
		}
		return "", fmt.Errorf("`%v %v` failed: %v %v: %w", r.handler.RuntimePath, strings.Join(runtimeArgs, " "), stderr.String(), stdout.String(), err)
	}

	return stdout.String(), nil
}

func (r *runtimeOCI) defaultRuntimeArgs() []string {
	args := []string{rootFlag, r.root}
	if r.config.CgroupManager().IsSystemd() {
		args = append(args, "--systemd-cgroup")
	}
	return args
}

// CheckpointContainer checkpoints a container.
func (r *runtimeOCI) CheckpointContainer(ctx context.Context, c *Container, specgen *rspec.Spec, leaveRunning bool) error {
	c.opLock.Lock()
	defer c.opLock.Unlock()
	runtimePath := c.RuntimePathForPlatform(r)
	if err := r.checkpointRestoreSupported(runtimePath); err != nil {
		return err
	}

	// Once CRIU infects the process in the container with the
	// parasite, the parasite also wants to write to the log
	// file which is outside of the container. Giving the log file
	// the label of the container enables logging for the parasite.
	if err := crutils.CRCreateFileWithLabel(
		c.Dir(),
		metadata.DumpLogFile,
		specgen.Linux.MountLabel,
	); err != nil {
		return err
	}

	// workPath will be used to store dump.log and stats-dump
	workPath := c.Dir()
	// imagePath is used by CRIU to store the actual checkpoint files
	imagePath := c.CheckpointPath()

	log.Debugf(ctx, "Writing checkpoint to %s", imagePath)
	log.Debugf(ctx, "Writing checkpoint logs to %s", workPath)
	args := []string{}
	args = append(
		args,
		"checkpoint",
		"--file-locks",
		"--image-path",
		imagePath,
		"--work-path",
		workPath,
	)
	if leaveRunning {
		args = append(args, "--leave-running")
	}

	args = append(args, c.ID())

	_, err := r.runtimeCmd(args...)
	if err != nil {
		return fmt.Errorf("running %q %q failed: %w", runtimePath, args, err)
	}

	c.SetCheckpointedAt(time.Now())
	if !leaveRunning {
		c.state.Status = ContainerStateStopped
		c.state.ExitCode = utils.Int32Ptr(0)
		c.state.Finished = c.CheckpointedAt()
	}

	return nil
}

// RestoreContainer restores a container.
func (r *runtimeOCI) RestoreContainer(ctx context.Context, c *Container, cgroupParent, mountLabel string) error {
	if err := r.checkpointRestoreSupported(c.RuntimePathForPlatform(r)); err != nil {
		return err
	}

	// Let's try to stat() CRIU's inventory file. If it does not exist, it makes
	// no sense to try a restore. This is a minimal check if a checkpoint exist.
	if _, err := os.Stat(filepath.Join(c.CheckpointPath(), "inventory.img")); os.IsNotExist(err) {
		return fmt.Errorf("a complete checkpoint for this container cannot be found, cannot restore: %w", err)
	}

	// remove conmon files
	attachFile := filepath.Join(c.BundlePath(), "attach")
	if err := os.Remove(attachFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error removing container %s attach file: %w", c.ID(), err)
	}

	ctlFile := filepath.Join(c.BundlePath(), "ctl")
	if err := os.Remove(ctlFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error removing container %s ctl file: %w", c.ID(), err)
	}

	winszFile := filepath.Join(c.BundlePath(), "winsz")
	if err := os.Remove(winszFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error removing container %s winsz file: %w", c.ID(), err)
	}

	c.state.InitPid = 0
	c.state.InitStartTime = ""

	// It is possible to tell runc to place the CRIU log files
	// at a custom location '--work-path'. But for restoring a
	// container we are not calling runc directly but conmon, which
	// then calls runc. It would be possible to change conmon to
	// also have the log file in the same location as during
	// checkpointing, but it is not really that important right now.
	if err := crutils.CRCreateFileWithLabel(
		c.BundlePath(),
		metadata.RestoreLogFile,
		mountLabel,
	); err != nil {
		return err
	}

	if err := r.CreateContainer(ctx, c, cgroupParent, true); err != nil {
		return err
	}

	// Once the container is restored, update the metadata
	// 1. Container is running again
	c.state.Status = ContainerStateRunning
	// 2. Update PID of the container (without that stopping will fail)
	pid, err := ReadConmonPidFile(c)
	if err != nil {
		return err
	}
	c.state.Pid = pid
	// 3. Reset ExitCode (also needed for stopping)
	c.state.ExitCode = nil
	// 4. Set start time (also restore time)
	c.state.Started = time.Now()

	return nil
}

func (r *runtimeOCI) checkpointRestoreSupported(runtimePath string) error {
	if err := criu.CheckForCriu(criu.PodCriuVersion); err != nil {
		return fmt.Errorf("check for CRIU %w", err)
	}
	if !crutils.CRRuntimeSupportsCheckpointRestore(runtimePath) {
		return errors.New("configured runtime does not support checkpoint/restore")
	}
	return nil
}
