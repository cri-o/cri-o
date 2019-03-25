// +build !linux

package oci

import (
	"os"
	"syscall"
)

func (r *Runtime) createContainerPlatform(c *Container, cgroupParent string, pid int) {
}

func sysProcAttrPlatform() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}

func newPipe() (parent *os.File, child *os.File, err error) {
	return os.Pipe()
}
