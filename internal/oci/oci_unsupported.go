//go:build !linux
// +build !linux

package oci

import (
	"errors"
	"os"
	"syscall"
)

func (r *Runtime) createContainerPlatform(c *Container, cgroupParent string, pid int) error {
	return errors.New("not implemented")
}

func sysProcAttrPlatform() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}

func newPipe() (*os.File, *os.File, error) {
	return os.Pipe()
}
