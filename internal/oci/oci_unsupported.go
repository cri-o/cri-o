//go:build !linux

package oci

import (
	"context"
	"os"
	"os/exec"
	"syscall"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

const InfraContainerName = "POD"

func (r *Runtime) createContainerPlatform(c *Container, cgroupParent string, pid int) error {
	return nil
}

func sysProcAttrPlatform() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}

func newPipe() (*os.File, *os.File, error) {
	return os.Pipe()
}

func (r *runtimeOCI) containerStats(ctr *Container, cgroup string) (*types.ContainerStats, error) {
	return nil, nil
}

// CleanupConmonCgroup cleans up conmon's group when using cgroupfs.
func (c *Container) CleanupConmonCgroup(ctx context.Context) {
}

// SetSeccompProfilePath sets the seccomp profile path
func (c *Container) SetSeccompProfilePath(pp string) {
}

// SeccompProfilePath returns the seccomp profile path
func (c *Container) SeccompProfilePath() string {
	return ""
}

// setSysProcAttr is a no-op on non-Linux platforms.
func setSysProcAttr(_ *exec.Cmd, _ uintptr) {
}
