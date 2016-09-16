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

	"github.com/Sirupsen/logrus"
	"github.com/kubernetes-incubator/ocid/utils"
	"github.com/opencontainers/runtime-spec/specs-go"
)

// New creates a new Runtime with options provided
func New(runtimePath string, containerDir string) (*Runtime, error) {
	r := &Runtime{
		name:         filepath.Base(runtimePath),
		path:         runtimePath,
		containerDir: containerDir,
	}
	return r, nil
}

// Runtime stores the information about a oci runtime
type Runtime struct {
	name         string
	path         string
	containerDir string
}

// syncInfo is used to return data from monitor process to daemon
type syncInfo struct {
	Pid int `"json:pid"`
}

// Name returns the name of the OCI Runtime
func (r *Runtime) Name() string {
	return r.name
}

// Path returns the full path the OCI Runtime executable
func (r *Runtime) Path() string {
	return r.path
}

// ContainerDir returns the path to the base directory for storing container configurations
func (r *Runtime) ContainerDir() string {
	return r.containerDir
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
func (r *Runtime) CreateContainer(c *Container) error {
	parentPipe, childPipe, err := newPipe()
	if err != nil {
		return fmt.Errorf("error creating socket pair: %v", err)
	}
	defer parentPipe.Close()

	args := []string{"-c", c.name}
	if c.terminal {
		args = append(args, "-t")
	}

	cmd := exec.Command("conmon", args...)
	cmd.Dir = c.bundlePath
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.ExtraFiles = append(cmd.ExtraFiles, childPipe)
	// 0, 1 and 2 are stdin, stdout and stderr
	cmd.Env = append(cmd.Env, fmt.Sprintf("_OCI_SYNCPIPE=%d", 3))

	err = cmd.Start()
	if err != nil {
		childPipe.Close()
		return err
	}

	// We don't need childPipe on the parent side
	childPipe.Close()

	// Wait to get container pid from conmon
	// TODO(mrunalp): Add a timeout here
	var si *syncInfo
	if err := json.NewDecoder(parentPipe).Decode(&si); err != nil {
		return fmt.Errorf("reading pid from init pipe: %v", err)
	}
	logrus.Infof("Received container pid: %v", si.Pid)
	return nil
}

// StartContainer starts a container.
func (r *Runtime) StartContainer(c *Container) error {
	if err := utils.ExecCmdWithStdStreams(os.Stdin, os.Stdout, os.Stderr, r.path, "start", c.name); err != nil {
		return err
	}
	c.state.Started = time.Now()
	return nil
}

// StopContainer stops a container.
func (r *Runtime) StopContainer(c *Container) error {
	// TODO: Check if it is still running after some time and send SIGKILL
	return utils.ExecCmdWithStdStreams(os.Stdin, os.Stdout, os.Stderr, r.path, "kill", c.name)
}

// DeleteContainer deletes a container.
func (r *Runtime) DeleteContainer(c *Container) error {
	return utils.ExecCmdWithStdStreams(os.Stdin, os.Stdout, os.Stderr, r.path, "delete", c.name)
}

// updateStatus refreshes the status of the container.
func (r *Runtime) UpdateStatus(c *Container) error {
	c.stateLock.Lock()
	defer c.stateLock.Unlock()
	out, err := exec.Command(r.path, "state", c.name).Output()
	if err != nil {
		return fmt.Errorf("error getting container state for %s: %s", c.name, err)
	}
	stateReader := bytes.NewReader(out)
	if err := json.NewDecoder(stateReader).Decode(&c.state); err != nil {
		return fmt.Errorf("failed to decode container status for %s: %s", c.name, err)
	}

	if c.state.Status == "stopped" {
		exitFilePath := filepath.Join(c.bundlePath, "exit")
		fi, err := os.Stat(exitFilePath)
		if err != nil {
			return fmt.Errorf("failed to find container exit file: %v", err)
		}
		st := fi.Sys().(*syscall.Stat_t)
		c.state.Finished = time.Unix(int64(st.Ctim.Sec), int64(st.Ctim.Nsec))

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

	return nil
}

// ContainerStatus returns the state of a container.
func (r *Runtime) ContainerStatus(c *Container) *ContainerState {
	c.stateLock.Lock()
	defer c.stateLock.Unlock()
	return c.state
}

// Container respresents a runtime container.
type Container struct {
	name       string
	bundlePath string
	logPath    string
	labels     map[string]string
	sandbox    string
	terminal   bool
	state      *ContainerState
	stateLock  sync.Mutex
}

// ContainerStatus represents the status of a container.
type ContainerState struct {
	specs.State
	Created  time.Time `json:"created"`
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished"`
	ExitCode int32     `json:"exitCode"`
}

// NewContainer creates a container object.
func NewContainer(name string, bundlePath string, logPath string, labels map[string]string, sandbox string, terminal bool) (*Container, error) {
	c := &Container{
		name:       name,
		bundlePath: bundlePath,
		logPath:    logPath,
		labels:     labels,
		sandbox:    sandbox,
		terminal:   terminal,
	}
	return c, nil
}

// Name returns the name of the container.
func (c *Container) Name() string {
	return c.name
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

// Sandbox returns the sandbox name of the container.
func (c *Container) Sandbox() string {
	return c.sandbox
}

// NetNsPath returns the path to the network namespace of the container.
func (c *Container) NetNsPath() (string, error) {
	if c.state == nil {
		return "", fmt.Errorf("container state is not populated")
	}
	return fmt.Sprintf("/proc/%d/ns/net", c.state.Pid), nil
}

// newPipe creates a unix socket pair for communication
func newPipe() (parent *os.File, child *os.File, err error) {
	fds, err := syscall.Socketpair(syscall.AF_LOCAL, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	return os.NewFile(uintptr(fds[1]), "parent"), os.NewFile(uintptr(fds[0]), "child"), nil
}
