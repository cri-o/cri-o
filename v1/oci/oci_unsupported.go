// +build !linux

package oci

import (
	"os"
	"syscall"

	"github.com/pkg/errors"
)

func (r *Runtime) createContainerPlatform(c *Container, cgroupParent string, pid int) error {
	return errors.Errorf("not implemented")
}

func sysProcAttrPlatform() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}

func newPipe() (*os.File, *os.File, error) {
	return os.Pipe()
}
