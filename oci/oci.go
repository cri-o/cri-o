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
	"sync"
	"syscall"
	"time"

	specs "github.com/opencontainers/runtime-spec/specs-go"

	"github.com/Sirupsen/logrus"
	"github.com/containernetworking/cni/pkg/ns"
	"github.com/kubernetes-incubator/cri-o/utils"
	"golang.org/x/sys/unix"
	"k8s.io/apimachinery/pkg/fields"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

const (
	// ContainerStateCreated represents the created state of a container
	ContainerStateCreated = "created"
	// ContainerStateRunning represents the running state of a container
	ContainerStateRunning = "running"
	// ContainerStateStopped represents the stopped state of a container
	ContainerStateStopped = "stopped"
	// ContainerCreateTimeout represents the value of container creating timeout
	ContainerCreateTimeout = 10 * time.Second
)

// New creates a new Runtime with options provided
func New(runtimePath string, runtimeHostPrivilegedPath string, conmonPath string, conmonEnv []string, cgroupManager string) (*Runtime, error) {
	r := &Runtime{
		name:           filepath.Base(runtimePath),
		path:           runtimePath,
		privilegedPath: runtimeHostPrivilegedPath,
		conmonPath:     conmonPath,
		conmonEnv:      conmonEnv,
		cgroupManager:  cgroupManager,
	}
	return r, nil
}

// Runtime stores the information about a oci runtime
type Runtime struct {
	name           string
	path           string
	privilegedPath string
	conmonPath     string
	conmonEnv      []string
	cgroupManager  string
}

// syncInfo is used to return data from monitor process to daemon
type syncInfo struct {
	Pid int `json:"pid"`
}

// exitCodeInfo is used to return the monitored process exit code to the daemon
type exitCodeInfo struct {
	ExitCode int32 `json:"exit_code"`
}

// Name returns the name of the OCI Runtime
func (r *Runtime) Name() string {
	return r.name
}

// Path returns the full path the OCI Runtime executable.
// Depending if the container is privileged, it will return
// the privileged runtime or not.
func (r *Runtime) Path(c *Container) string {
	if c.privileged && r.privilegedPath != "" {
		return r.privilegedPath
	}

	return r.path
}

// Version returns the version of the OCI Runtime
func (r *Runtime) Version() (string, error) {
	runtimeVersion, err := getOCIVersion(r.path, "-v")
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
func (r *Runtime) CreateContainer(c *Container, cgroupParent string) error {
	parentPipe, childPipe, err := newPipe()
	if err != nil {
		return fmt.Errorf("error creating socket pair: %v", err)
	}
	defer parentPipe.Close()

	var args []string
	if r.cgroupManager == "systemd" {
		args = append(args, "-s")
	}
	args = append(args, "-c", c.name)
	args = append(args, "-r", r.Path(c))
	args = append(args, "-b", c.bundlePath)
	args = append(args, "-p", filepath.Join(c.bundlePath, "pidfile"))
	args = append(args, "-l", c.logPath)
	if c.terminal {
		args = append(args, "-t")
	}
	logrus.WithFields(logrus.Fields{
		"args": args,
	}).Debugf("running conmon: %s", r.conmonPath)

	cmd := exec.Command(r.conmonPath, args...)
	cmd.Dir = c.bundlePath
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.ExtraFiles = append(cmd.ExtraFiles, childPipe)
	// 0, 1 and 2 are stdin, stdout and stderr
	cmd.Env = append(r.conmonEnv, fmt.Sprintf("_OCI_SYNCPIPE=%d", 3))

	err = cmd.Start()
	if err != nil {
		childPipe.Close()
		return err
	}

	// We don't need childPipe on the parent side
	childPipe.Close()

	// Move conmon to specified cgroup
	if cgroupParent != "" {
		if r.cgroupManager == "systemd" {
			logrus.Infof("Running conmon under slice %s and unitName %s", cgroupParent, createUnitName("ocid", c.name))
			if err = utils.RunUnderSystemdScope(cmd.Process.Pid, cgroupParent, createUnitName("ocid", c.name)); err != nil {
				logrus.Warnf("Failed to add conmon to sandbox cgroup: %v", err)
			}
		}
	}

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
			return err
		}
		logrus.Infof("Received container pid: %d", ss.si.Pid)
	case <-time.After(ContainerCreateTimeout):
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
	if err := utils.ExecCmdWithStdStreams(os.Stdin, os.Stdout, os.Stderr, r.Path(c), "start", c.name); err != nil {
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

	logFile, err := ioutil.TempFile("", "ocid-log-"+c.name)
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

	var args []string
	args = append(args, "-c", c.name)
	args = append(args, "-r", r.Path(c))
	args = append(args, "-p", pidFile.Name())
	args = append(args, "-e")
	if c.terminal {
		args = append(args, "-t")
	}
	args = append(args, "-l", logPath)

	args = append(args, command...)

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

	if timeout > 0 {
		done := make(chan error, 1)
		go func() {
			done <- cmd.Wait()
		}()

		select {
		case <-time.After(time.Duration(timeout) * time.Second):
			err = unix.Kill(cmd.Process.Pid, syscall.SIGKILL)
			if err != nil && err != syscall.ESRCH {
				return nil, ExecSyncError{
					Stdout:   stdoutBuf,
					Stderr:   stderrBuf,
					ExitCode: -1,
					Err:      fmt.Errorf("failed to kill process on timeout: %+v", err),
				}
			}
			return nil, ExecSyncError{
				Stdout:   stdoutBuf,
				Stderr:   stderrBuf,
				ExitCode: -1,
				Err:      fmt.Errorf("command timed out"),
			}
		case err = <-done:
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
						return nil, ExecSyncError{
							Stdout:   stdoutBuf,
							Stderr:   stderrBuf,
							ExitCode: int32(status.ExitStatus()),
							Err:      err,
						}
					}
				} else {
					return nil, ExecSyncError{
						Stdout:   stdoutBuf,
						Stderr:   stderrBuf,
						ExitCode: -1,
						Err:      err,
					}
				}
			}

		}
	} else {
		err = cmd.Wait()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
					return nil, ExecSyncError{
						Stdout:   stdoutBuf,
						Stderr:   stderrBuf,
						ExitCode: int32(status.ExitStatus()),
						Err:      err,
					}
				}
			} else {
				return nil, ExecSyncError{
					Stdout:   stdoutBuf,
					Stderr:   stderrBuf,
					ExitCode: -1,
					Err:      err,
				}
			}
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

	logrus.Infof("Received container exit code: %v", ec.ExitCode)

	if ec.ExitCode != 0 {
		return nil, ExecSyncError{
			Stdout:   stdoutBuf,
			Stderr:   stderrBuf,
			ExitCode: ec.ExitCode,
			Err:      fmt.Errorf("container workload exited with error %v", ec.ExitCode),
		}
	}

	// The actual logged output is not the same as stdoutBuf and stderrBuf,
	// which are used for getting error information. For the actual
	// ExecSyncResponse we have to read the logfile.
	// XXX: Currently runC dups the same console over both stdout and stderr,
	//      so we can't differentiate between the two.

	outputBytes, err := ioutil.ReadFile(logPath)
	if err != nil {
		return nil, ExecSyncError{
			Stdout:   stdoutBuf,
			Stderr:   stderrBuf,
			ExitCode: -1,
			Err:      err,
		}
	}

	return &ExecSyncResponse{
		Stdout:   outputBytes,
		Stderr:   nil,
		ExitCode: 0,
	}, nil
}

// StopContainer stops a container.
func (r *Runtime) StopContainer(c *Container) error {
	c.opLock.Lock()
	defer c.opLock.Unlock()
	if err := utils.ExecCmdWithStdStreams(os.Stdin, os.Stdout, os.Stderr, r.Path(c), "kill", c.name, "TERM"); err != nil {
		return err
	}
	i := 0
	for {
		if i == 1000 {
			err := unix.Kill(c.state.Pid, syscall.SIGKILL)
			if err != nil && err != syscall.ESRCH {
				return fmt.Errorf("failed to kill process: %v", err)
			}
			break
		}
		// Check if the process is still around
		err := unix.Kill(c.state.Pid, 0)
		if err == syscall.ESRCH {
			break
		}
		time.Sleep(10 * time.Millisecond)
		i++
	}

	return nil
}

// DeleteContainer deletes a container.
func (r *Runtime) DeleteContainer(c *Container) error {
	c.opLock.Lock()
	defer c.opLock.Unlock()
	return utils.ExecCmdWithStdStreams(os.Stdin, os.Stdout, os.Stderr, r.Path(c), "delete", c.name)
}

// UpdateStatus refreshes the status of the container.
func (r *Runtime) UpdateStatus(c *Container) error {
	c.opLock.Lock()
	defer c.opLock.Unlock()
	out, err := exec.Command(r.Path(c), "state", c.name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("error getting container state for %s: %s: %q", c.name, err, out)
	}
	if err := json.NewDecoder(bytes.NewBuffer(out)).Decode(&c.state); err != nil {
		return fmt.Errorf("failed to decode container status for %s: %s", c.name, err)
	}

	if c.state.Status == ContainerStateStopped {
		exitFilePath := filepath.Join(c.bundlePath, "exit")
		fi, err := os.Stat(exitFilePath)
		if err != nil {
			logrus.Warnf("failed to find container exit file: %v", err)
			c.state.ExitCode = -1
		} else {
			st := fi.Sys().(*syscall.Stat_t)
			c.state.Finished = time.Unix(st.Ctim.Sec, st.Ctim.Nsec)

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
	}

	return nil
}

// ContainerStatus returns the state of a container.
func (r *Runtime) ContainerStatus(c *Container) *ContainerState {
	c.opLock.Lock()
	defer c.opLock.Unlock()
	return c.state
}

// Container respresents a runtime container.
type Container struct {
	id          string
	name        string
	bundlePath  string
	logPath     string
	labels      fields.Set
	annotations fields.Set
	image       *pb.ImageSpec
	sandbox     string
	netns       ns.NetNS
	terminal    bool
	privileged  bool
	state       *ContainerState
	metadata    *pb.ContainerMetadata
	opLock      sync.Mutex
}

// ContainerState represents the status of a container.
type ContainerState struct {
	specs.State
	Created  time.Time `json:"created"`
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished"`
	ExitCode int32     `json:"exitCode"`
}

// NewContainer creates a container object.
func NewContainer(id string, name string, bundlePath string, logPath string, netns ns.NetNS, labels map[string]string, annotations map[string]string, image *pb.ImageSpec, metadata *pb.ContainerMetadata, sandbox string, terminal bool, privileged bool) (*Container, error) {
	c := &Container{
		id:          id,
		name:        name,
		bundlePath:  bundlePath,
		logPath:     logPath,
		labels:      labels,
		sandbox:     sandbox,
		netns:       netns,
		terminal:    terminal,
		privileged:  privileged,
		metadata:    metadata,
		annotations: annotations,
		image:       image,
	}
	return c, nil
}

// Name returns the name of the container.
func (c *Container) Name() string {
	return c.name
}

// ID returns the id of the container.
func (c *Container) ID() string {
	return c.id
}

// BundlePath returns the bundlePath of the container.
func (c *Container) BundlePath() string {
	return c.bundlePath
}

// LogPath returns the log path of the container.
func (c *Container) LogPath() string {
	return c.logPath
}

// Labels returns the labels of the container.
func (c *Container) Labels() map[string]string {
	return c.labels
}

// Annotations returns the annotations of the container.
func (c *Container) Annotations() map[string]string {
	return c.annotations
}

// Image returns the image of the container.
func (c *Container) Image() *pb.ImageSpec {
	return c.image
}

// Sandbox returns the sandbox name of the container.
func (c *Container) Sandbox() string {
	return c.sandbox
}

// NetNsPath returns the path to the network namespace of the container.
func (c *Container) NetNsPath() (string, error) {
	if c.state == nil {
		return "", fmt.Errorf("container state is not populated")
	}

	if c.netns == nil {
		return fmt.Sprintf("/proc/%d/ns/net", c.state.Pid), nil
	}

	return c.netns.Path(), nil
}

// Metadata returns the metadata of the container.
func (c *Container) Metadata() *pb.ContainerMetadata {
	return c.metadata
}

// newPipe creates a unix socket pair for communication
func newPipe() (parent *os.File, child *os.File, err error) {
	fds, err := syscall.Socketpair(syscall.AF_LOCAL, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	return os.NewFile(uintptr(fds[1]), "parent"), os.NewFile(uintptr(fds[0]), "child"), nil
}

// RuntimeReady checks if the runtime is up and ready to accept
// basic containers e.g. container only needs host network.
func (r *Runtime) RuntimeReady() (bool, error) {
	return true, nil
}

// NetworkReady checks if the runtime network is up and ready to
// accept containers which require container network.
func (r *Runtime) NetworkReady() (bool, error) {
	return true, nil
}
