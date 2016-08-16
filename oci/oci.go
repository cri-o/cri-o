package oci

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mrunalp/ocid/utils"
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
	sandboxDir   string
	containerDir string
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
	return utils.ExecCmdWithStdStreams(os.Stdin, os.Stdout, os.Stderr, r.path, "--systemd-cgroup", "create", "--bundle", c.bundlePath, c.name)
}

// StartContainer starts a container.
func (r *Runtime) StartContainer(c *Container) error {
	return utils.ExecCmdWithStdStreams(os.Stdin, os.Stdout, os.Stderr, r.path, "start", c.name)
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

// Container respresents a runtime container.
type Container struct {
	name       string
	bundlePath string
	logPath    string
	labels     map[string]string
	sandbox    string
}

// NewContainer creates a container object.
func NewContainer(name string, bundlePath string, logPath string, labels map[string]string, sandbox string) (*Container, error) {
	c := &Container{
		name:       name,
		bundlePath: bundlePath,
		logPath:    logPath,
		labels:     labels,
		sandbox:    sandbox,
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
