package oci

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kubernetes-sigs/cri-o/pkg/findprocess"
	"github.com/kubernetes-sigs/cri-o/utils"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	kwait "k8s.io/apimachinery/pkg/util/wait"
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

	// BufSize is the size of buffers passed in to socekts
	BufSize = 8192

	// killContainerTimeout is the timeout that we wait for the container to
	// be SIGKILLed.
	killContainerTimeout = 2 * time.Minute

	// minCtrStopTimeout is the minimal amout of time in seconds to wait
	// before issuing a timeout regarding the proper termination of the
	// container.
	minCtrStopTimeout = 10

	// UntrustedRuntime is the implicit runtime handler name used to
	// fallback to the untrusted runtime.
	UntrustedRuntime = "untrusted"
)

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
	ctrStopTimeout int64) (*Runtime, error) {
	if runtimeTrustedPath == "" {
		// this means no "runtime" key in config as it's deprecated, fallback to
		// the runtime handler configured as default.
		r, ok := runtimes[defaultRuntime]
		if !ok {
			return nil, fmt.Errorf("no runtime configured for default_runtime=%q", defaultRuntime)
		}
		runtimeTrustedPath = r.RuntimePath
	}
	r := &Runtime{
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
	return r, nil
}

// Runtime stores the information about a oci runtime
type Runtime struct {
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

// RuntimeHandler represents each item of the "crio.runtime.runtimes" TOML
// config table.
type RuntimeHandler struct {
	RuntimePath string `toml:"runtime_path"`
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

// Name returns the name of the OCI Runtime
func (r *Runtime) Name() string {
	return r.name
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

// Path returns the full path the OCI Runtime executable.
// Depending if the container is privileged and/or trusted,
// this will return either the trusted or untrusted runtime path.
func (r *Runtime) Path(c *Container) (string, error) {
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

// Version returns the version of the OCI Runtime
func (r *Runtime) Version() (string, error) {
	runtimeVersion, err := getOCIVersion(r.trustedPath, "-v")
	if err != nil {
		return "", err
	}
	return runtimeVersion, nil
}

func getOCIVersion(name string, args ...string) (string, error) {
	out, err := utils.ExecCmd(name, args...)
	if err != nil {
		return "", err
	}

	firstLine := out[:strings.Index(out, "\n")]
	v := firstLine[strings.LastIndex(firstLine, " ")+1:]
	return v, nil
}

// CreateContainer creates a container.
func (r *Runtime) CreateContainer(c *Container, cgroupParent string) (err error) {
	var stderrBuf bytes.Buffer
	parentPipe, childPipe, err := newPipe()
	childStartPipe, parentStartPipe, err := newPipe()
	if err != nil {
		return fmt.Errorf("error creating socket pair: %v", err)
	}
	defer parentPipe.Close()
	defer parentStartPipe.Close()

	var args []string
	if r.cgroupManager == SystemdCgroupsManager {
		args = append(args, "-s")
	}
	if r.cgroupManager == CgroupfsCgroupsManager {
		args = append(args, "--syslog")
	}

	rPath, err := r.Path(c)
	if err != nil {
		return err
	}

	args = append(args, "-c", c.id)
	args = append(args, "-u", c.id)
	args = append(args, "-r", rPath)
	args = append(args, "-b", c.bundlePath)
	args = append(args, "-p", filepath.Join(c.bundlePath, "pidfile"))
	args = append(args, "-l", c.logPath)
	args = append(args, "--exit-dir", r.containerExitsDir)
	args = append(args, "--socket-dir-path", r.containerAttachSocketDir)
	args = append(args, "--log-level", logrus.GetLevel().String())
	if r.logSizeMax >= 0 {
		args = append(args, "--log-size-max", fmt.Sprintf("%v", r.logSizeMax))
	}
	if r.noPivot {
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
	}).Debugf("running conmon: %s", r.conmonPath)

	cmd := exec.Command(r.conmonPath, args...)
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
	cmd.Env = append(r.conmonEnv, fmt.Sprintf("_OCI_SYNCPIPE=%d", 3))
	cmd.Env = append(cmd.Env, fmt.Sprintf("_OCI_STARTPIPE=%d", 4))
	cmd.Env = append(cmd.Env, fmt.Sprintf("XDG_RUNTIME_DIR=%s", os.Getenv("XDG_RUNTIME_DIR")))

	err = cmd.Start()
	if err != nil {
		childPipe.Close()
		return err
	}

	// We don't need childPipe on the parent side
	childPipe.Close()
	childStartPipe.Close()

	// Platform specific container setup
	if err := r.createContainerPlatform(c, cgroupParent, cmd.Process.Pid); err != nil {
		logrus.Warnf("%s", err)
	}

	/* We set the cgroup, now the child can start creating children */
	someData := []byte{0}
	_, err = parentStartPipe.Write(someData)
	if err != nil {
		cmd.Wait()
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
			r.DeleteContainer(c)
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

func createUnitName(prefix string, name string) string {
	return fmt.Sprintf("%s-%s.scope", prefix, name)
}

// StartContainer starts a container.
func (r *Runtime) StartContainer(c *Container) error {
	c.opLock.Lock()
	defer c.opLock.Unlock()

	rPath, err := r.Path(c)
	if err != nil {
		return err
	}

	if err := utils.ExecCmdWithStdStreams(os.Stdin, os.Stdout, os.Stderr, rPath, "start", c.id); err != nil {
		return err
	}
	c.state.Started = time.Now()
	return nil
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
			if content[contentLen-1] == '\n' {
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

// ExecSync execs a command in a container and returns it's stdout, stderr and return code.
func (r *Runtime) ExecSync(c *Container, command []string, timeout int64) (resp *ExecSyncResponse, err error) {
	pidFile, parentPipe, childPipe, err := prepareExec()
	if err != nil {
		return nil, ExecSyncError{
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
		return nil, ExecSyncError{
			ExitCode: -1,
			Err:      err,
		}
	}
	logPath := logFile.Name()
	defer func() {
		logFile.Close()
		os.RemoveAll(logPath)
	}()

	rPath, err := r.Path(c)
	if err != nil {
		return nil, ExecSyncError{
			ExitCode: -1,
			Err:      err,
		}
	}

	var args []string
	args = append(args, "-c", c.id)
	args = append(args, "-r", rPath)
	args = append(args, "-p", pidFile.Name())
	args = append(args, "-e")
	if c.terminal {
		args = append(args, "-t")
	}
	if timeout > 0 {
		args = append(args, "-T")
		args = append(args, fmt.Sprintf("%d", timeout))
	}
	args = append(args, "-l", logPath)
	args = append(args, "--socket-dir-path", ContainerAttachSocketDir)
	args = append(args, "--log-level", logrus.GetLevel().String())

	processFile, err := PrepareProcessExec(c, command, c.terminal)
	if err != nil {
		return nil, ExecSyncError{
			ExitCode: -1,
			Err:      err,
		}
	}
	defer os.RemoveAll(processFile.Name())

	args = append(args, "--exec-process-spec", processFile.Name())

	cmd := exec.Command(r.conmonPath, args...)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	cmd.ExtraFiles = append(cmd.ExtraFiles, childPipe)
	// 0, 1 and 2 are stdin, stdout and stderr
	cmd.Env = append(r.conmonEnv, fmt.Sprintf("_OCI_SYNCPIPE=%d", 3))

	err = cmd.Start()
	if err != nil {
		childPipe.Close()
		return nil, ExecSyncError{
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
		return nil, ExecSyncError{
			Stdout:   stdoutBuf,
			Stderr:   stderrBuf,
			ExitCode: getExitCode(err),
			Err:      err,
		}
	}

	var ec *exitCodeInfo
	if err := json.NewDecoder(parentPipe).Decode(&ec); err != nil {
		return nil, ExecSyncError{
			Stdout:   stdoutBuf,
			Stderr:   stderrBuf,
			ExitCode: -1,
			Err:      err,
		}
	}

	logrus.Debugf("Received container exit code: %v, message: %s", ec.ExitCode, ec.Message)

	if ec.ExitCode == -1 {
		return nil, ExecSyncError{
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
		return nil, ExecSyncError{
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
func (r *Runtime) UpdateContainer(c *Container, res *rspec.LinuxResources) error {
	rPath, err := r.Path(c)
	if err != nil {
		return err
	}

	cmd := exec.Command(rPath, "update", "--resources", "-", c.id)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
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

// WaitContainerStateStopped runs a loop polling UpdateStatus(), seeking for
// the container status to be updated to 'stopped'. Either it gets the expected
// status and returns nil, or it reaches the timeout and returns an error.
func (r *Runtime) WaitContainerStateStopped(ctx context.Context, c *Container) (err error) {
	// No need to go further and spawn the go routine if the container
	// is already in the expected status.
	if r.ContainerStatus(c).Status == ContainerStateStopped {
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
				if err := r.UpdateStatus(c); err != nil {
					done <- err
					close(done)
					return
				}
				if r.ContainerStatus(c).Status == ContainerStateStopped {
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

// StopContainer stops a container. Timeout is given in seconds.
func (r *Runtime) StopContainer(ctx context.Context, c *Container, timeout int64) error {
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

	rPath, err := r.Path(c)
	if err != nil {
		return err
	}

	if timeout > 0 {
		if err := utils.ExecCmdWithStdStreams(os.Stdin, os.Stdout, os.Stderr, rPath, "kill", c.id, c.GetStopSignal()); err != nil {
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

	if err := utils.ExecCmdWithStdStreams(os.Stdin, os.Stdout, os.Stderr, rPath, "kill", c.id, "KILL"); err != nil {
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
func (r *Runtime) DeleteContainer(c *Container) error {
	c.opLock.Lock()
	defer c.opLock.Unlock()

	rPath, err := r.Path(c)
	if err != nil {
		return err
	}

	_, err = utils.ExecCmd(rPath, "delete", "--force", c.id)
	return err
}

// SetStartFailed sets the container state appropriately after a start failure
func (r *Runtime) SetStartFailed(c *Container, err error) {
	c.opLock.Lock()
	defer c.opLock.Unlock()
	// adjust finished and started times
	c.state.Finished, c.state.Started = c.state.Created, c.state.Created
	c.state.Error = err.Error()
}

// UpdateStatus refreshes the status of the container.
func (r *Runtime) UpdateStatus(c *Container) error {
	c.opLock.Lock()
	defer c.opLock.Unlock()

	rPath, err := r.Path(c)
	if err != nil {
		return err
	}

	out, err := exec.Command(rPath, "state", c.id).Output()
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
		exitFilePath := filepath.Join(r.containerExitsDir, c.id)
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
			c.state.Finished = getFinishedTime(fi)
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

// ContainerStatus returns the state of a container.
func (r *Runtime) ContainerStatus(c *Container) *ContainerState {
	c.opLock.Lock()
	defer c.opLock.Unlock()
	return c.state
}

// CurrentContainerStatus returns the state of a container without using a lock.
func (r *Runtime) CurrentContainerStatus(c *Container) *ContainerState {
	return c.state
}

// PauseContainer pauses a container.
func (r *Runtime) PauseContainer(c *Container) error {
	c.opLock.Lock()
	defer c.opLock.Unlock()

	rPath, err := r.Path(c)
	if err != nil {
		return err
	}

	_, err = utils.ExecCmd(rPath, "pause", c.id)
	return err
}

// UnpauseContainer unpauses a container.
func (r *Runtime) UnpauseContainer(c *Container) error {
	c.opLock.Lock()
	defer c.opLock.Unlock()

	rPath, err := r.Path(c)
	if err != nil {
		return err
	}

	_, err = utils.ExecCmd(rPath, "resume", c.id)
	return err
}

// PrepareProcessExec returns the path of the process.json used in runc exec -p
// caller is responsible to close the returned *os.File if needed.
func PrepareProcessExec(c *Container, cmd []string, tty bool) (*os.File, error) {
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
