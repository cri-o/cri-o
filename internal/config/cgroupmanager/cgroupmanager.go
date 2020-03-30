// +build linux

package cgroupmanager

import (
	"github.com/pkg/errors"
)

const (
	cgroupMemorySubsystemMountPathV1 = "/sys/fs/cgroup/memory"
	cgroupMemorySubsystemMountPathV2 = "/sys/fs/cgroup"
	scopePrefix                      = "crio"
	// minMemoryLimit is the minimum memory that must be set for a container.
	// A lower value would result in the container failing to start.
	minMemoryLimit = 12582912
	// CgroupfsCgroupManager represents cgroupfs native cgroup manager
	CgroupfsCgroupManager = "cgroupfs"
	// SystemdCgroupManager represents systemd native cgroup manager
	SystemdCgroupManager = "systemd"

	DefaultCgroupManager = SystemdCgroupManager
)

// CgroupManager is an interface to interact with cgroups on a node. CRI-O is configured at startup to either use
// systemd or cgroupfs, and the node itself is booted with cgroup v1, or cgroup v2. CgroupManager is an interface for
// the CRI-O server to use cgroups, regardless of how it or the node was configured.
type CgroupManager interface {
	// Name returns the name of the cgroup manager (either cgroupfs or systemd)
	Name() string
	// IsSystemd returns whether it is a systemd cgroup manager
	IsSystemd() bool
	// GetContainerCgroupPath takes arguments sandbox parent cgroup and container ID and returns
	// the cgroup path for that containerID. If parentCgroup is empty, it
	// uses the default parent for that particular manager
	GetContainerCgroupPath(string, string) string
	// GetSandboxCgroupPath takes the sandbox parent, and sandbox ID. It
	// returns the cgroup parent, cgroup path, and error. For systemd cgroups,
	// it also checks there is enough memory in the given cgroup (4mb is needed for the runtime)
	GetSandboxCgroupPath(string, string) (string, string, error)
	// MoveConmonToCgroup takes the container ID, cgroup parent, conmon's cgroup (from the config) and conmon's PID
	// It attempts to move conmon to the correct cgroup.
	// It returns the cgroupfs parent that conmon was put into
	// so that CRI-O can clean the parent cgroup of the newly added conmon once the process terminates (systemd handles this for us)
	MoveConmonToCgroup(cid, cgroupParent, conmonCgroup string, pid int) string
}

// New creates a new CgroupManager with defaults
func New() CgroupManager {
	// we can eat the error here because we control what the DefaultCgroupManager is
	cm, _ := SetCgroupManager(DefaultCgroupManager)
	return cm
}

// SetCgroupManager takes a string and branches on it to return
// the type of cgroup manager configured
func SetCgroupManager(cgroupManager string) (CgroupManager, error) {
	initializeCgroups()
	switch cgroupManager {
	case SystemdCgroupManager:
		if CgroupIsV2() {
			return new(Systemdv2Manager), nil
		}
		return new(Systemdv1Manager), nil
	case CgroupfsCgroupManager:
		return new(CgroupfsManager), nil
	default:
		return nil, errors.Errorf("invalid cgroup manager: %s", cgroupManager)
	}
}
