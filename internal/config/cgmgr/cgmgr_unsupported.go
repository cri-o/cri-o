// +build !linux

package cgmgr

import (
	"github.com/pkg/errors"
)

type CgroupManager interface {
	ContainerCgroupPath(string, string) string
	// cgroup parent, cgroup path, error
	SandboxCgroupPath(string, string) string
}

func InitializeCgroupManager(cgroupManager string) (CgroupManager, error) {
	return nil, errors.New("not implemented yet")
}
