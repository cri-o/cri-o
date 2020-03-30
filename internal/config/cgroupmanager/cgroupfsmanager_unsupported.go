// +build !linux

package cgroupmanager

import (
	"fmt"
)

type CgroupManager interface {
	GetContainerCgroupPath(string, string) string
	// cgroup parent, cgroup path, error
	GetSandboxCgroupPath(string, string) string
}

func InitializeCgroupManager(cgroupManager string) (CgroupManager, error) {
	return nil, fmt.Errorf("not implemented yet")
}
