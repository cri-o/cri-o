package oci

import (
	"path/filepath"
	"strings"

	"github.com/mrunalp/ocid/utils"
)

// New creates a new Runtime with options provided
func New(runtimePath string, sandboxDir string, containerDir string) (*Runtime, error) {
	r := &Runtime{
		name:         filepath.Base(runtimePath),
		path:         runtimePath,
		sandboxDir:   sandboxDir,
		containerDir: containerDir,
	}
	return r, nil
}

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

// SandboxDir returns the path to the base directory for storing sandbox configurations
func (r *Runtime) SandboxDir() string {
	return r.sandboxDir
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
