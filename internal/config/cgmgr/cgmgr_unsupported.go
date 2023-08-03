//go:build !linux
// +build !linux

package cgmgr

import (
	"errors"
)

type CgroupManager interface {
	// String returns the name of the cgroup manager (either cgroupfs or systemd)
	Name() string
	// IsSystemd returns whether it is a systemd cgroup manager
	IsSystemd() bool
	// ContainerCgroupPath takes arguments sandbox parent cgroup and container ID and returns
	// the cgroup path for that containerID. If parentCgroup is empty, it
	// uses the default parent for that particular manager
	ContainerCgroupPath(string, string) string
	// cgroup parent, cgroup path, error
	SandboxCgroupPath(string, string) string
	// RemoveSandboxCgroup takes the sandbox parent, and sandbox ID.
	// It removes the cgroup for that sandbox, which is useful when spoofing an infra container
	RemoveSandboxCgroup(sbParent, containerID string) error
}

type NullCgroupManager struct {
}

func InitializeCgroupManager(cgroupManager string) (CgroupManager, error) {
	return nil, errors.New("not implemented yet")
}

// New creates a new CgroupManager with defaults
func New() CgroupManager {
	return &NullCgroupManager{}
}

func SetCgroupManager(cgroupManager string) (CgroupManager, error) {
	return &NullCgroupManager{}, nil
}

// MoveProcessToContainerCgroup moves process to the container cgroup
func MoveProcessToContainerCgroup(containerPid, commandPid int) error {
	return nil
}

// VerifyMemoryIsEnough verifies that the cgroup memory limit is above a specified minimum memory limit.
func VerifyMemoryIsEnough(memoryLimit int64) error {
	return nil
}

func (*NullCgroupManager) Name() string {
	return "none"
}

func (*NullCgroupManager) IsSystemd() bool {
	return false
}

func (*NullCgroupManager) ContainerCgroupPath(string, string) string {
	return ""
}

func (*NullCgroupManager) SandboxCgroupPath(string, string) string {
	return ""
}

func (*NullCgroupManager) RemoveSandboxCgroup(sbParent, containerID string) error {
	return nil
}
