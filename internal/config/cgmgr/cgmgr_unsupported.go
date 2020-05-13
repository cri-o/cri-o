// +build !linux

package cgmgr

import (
	"github.com/pkg/errors"
)

type CgroupManager interface {
	GetContainerCgroupPath(string, string) string
	// cgroup parent, cgroup path, error
	GetSandboxCgroupPath(string, string) string
}

func InitializeCgroupManager(cgroupManager string) (CgroupManager, error) {
	return nil, errors.New("not implemented yet")
}
