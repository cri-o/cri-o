//go:build linux

package cgmgr

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/opencontainers/cgroups"
	"github.com/opencontainers/cgroups/manager"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"

	"github.com/cri-o/cri-o/internal/config/node"
	"github.com/cri-o/cri-o/internal/lib/stats"
)

const (
	CrioPrefix = "crio"
	// CgroupfsCgroupManager represents cgroupfs native cgroup manager.
	cgroupfsCgroupManager = "cgroupfs"
	// SystemdCgroupManager represents systemd native cgroup manager.
	systemdCgroupManager = "systemd"

	DefaultCgroupManager = systemdCgroupManager

	// these constants define the path and name of the memory max file
	// for v1 and v2 respectively.
	CgroupMemoryPathV1    = "/sys/fs/cgroup/memory"
	cgroupMemoryMaxFileV1 = "memory.limit_in_bytes"
	CgroupMemoryPathV2    = "/sys/fs/cgroup"
	cgroupMemoryMaxFileV2 = "memory.max"
)

// CgroupManager is an interface to interact with cgroups on a node. CRI-O is configured at startup to either use
// systemd or cgroupfs, and the node itself is booted with cgroup v1, or cgroup v2. CgroupManager is an interface for
// the CRI-O server to use cgroups, regardless of how it or the node was configured.
type CgroupManager interface {
	// String returns the name of the cgroup manager (either cgroupfs or systemd)
	Name() string
	// IsSystemd returns whether it is a systemd cgroup manager
	IsSystemd() bool
	// ContainerCgroupPath takes arguments sandbox parent cgroup and container ID and returns
	// the cgroup path for that containerID. If parentCgroup is empty, it
	// uses the default parent for that particular manager
	ContainerCgroupPath(string, string) string
	// ContainerCgroupAbsolutePath takes arguments sandbox parent cgroup and container ID and
	// returns the cgroup path on disk for that containerID. If parentCgroup is empty, it
	// uses the default parent for that particular manager
	ContainerCgroupAbsolutePath(string, string) (string, error)
	// ContainerCgroupManager takes the cgroup parent, and container ID.
	// It returns the raw libcontainer cgroup manager for that container.
	ContainerCgroupManager(sbParent, containerID string) (cgroups.Manager, error)
	// RemoveContainerCgManager removes the cgroup manager for the container
	RemoveContainerCgManager(containerID string)
	// ContainerCgroupStats takes the sandbox parent, and container ID.
	// It creates a new cgroup if one does not already exist.
	// It returns the cgroup stats for that container.
	ContainerCgroupStats(sbParent, containerID string) (*stats.CgroupStats, error)
	// SandboxCgroupPath takes the sandbox parent, and sandbox ID, and container minimum memory. It
	// returns the cgroup parent, cgroup path, and error. For systemd cgroups,
	// it also checks there is enough memory in the given cgroup
	SandboxCgroupPath(string, string, int64) (string, string, error)
	// SandboxCgroupManager takes the cgroup parent, and sandbox ID.
	// It returns the raw libcontainer cgroup manager for that sandbox.
	SandboxCgroupManager(sbParent, sbID string) (cgroups.Manager, error)
	// RemoveSandboxCgroupManager removes the cgroup manager for the sandbox
	RemoveSandboxCgManager(sbID string)
	// MoveConmonToCgroup takes the container ID, cgroup parent, conmon's cgroup (from the config), conmon's PID, and some customized resources
	// It attempts to move conmon to the correct cgroup, and set the resources for that cgroup.
	// It returns the cgroupfs parent that conmon was put into
	// so that CRI-O can clean the parent cgroup of the newly added conmon once the process terminates (systemd handles this for us)
	MoveConmonToCgroup(cid, cgroupParent, conmonCgroup string, pid int, resources *rspec.LinuxResources) (string, error)
	// CreateSandboxCgroup takes the sandbox parent, and sandbox ID.
	// It creates a new cgroup for that sandbox, which is useful when spoofing an infra container.
	CreateSandboxCgroup(sbParent, containerID string) error
	// RemoveSandboxCgroup takes the sandbox parent, and sandbox ID.
	// It removes the cgroup for that sandbox, which is useful when spoofing an infra container.
	RemoveSandboxCgroup(sbParent, containerID string) error
	// SandboxCgroupStats takes the sandbox parent, and sandbox ID.
	// It creates a new cgroup for that sandbox if it does not already exist.
	// It returns the cgroup stats for that sandbox.
	SandboxCgroupStats(sbParent, sbID string) (*stats.CgroupStats, error)
	// ExecCgroupManager returns the cgroup manager for the exec cgroup used to place exec processes.
	// The cgroupPath parameter is the container's cgroup path from spec.Linux.CgroupsPath.
	// This is only supported on cgroup v2.
	ExecCgroupManager(cgroupPath string) (cgroups.Manager, error)
	// PodAndContainerCgroupManagers returns the libcontainer cgroup managers for both the pod and container cgroups.
	// The sbParent is the sandbox parent cgroup, and containerID is the container's ID.
	// It returns:
	//   - podManager: the cgroup manager for the pod cgroup
	//   - containerManagers: a slice of cgroup managers for the container cgroup(s).
	//     This may include an extra manager if crun creates a sub-cgroup of the container.
	PodAndContainerCgroupManagers(sbParent, containerID string) (podManager cgroups.Manager, containerManagers []cgroups.Manager, err error)
}

// New creates a new CgroupManager with defaults.
func New() CgroupManager {
	cm, err := SetCgroupManager(DefaultCgroupManager)
	if err != nil {
		panic(err)
	}

	return cm
}

// SetCgroupManager takes a string and branches on it to return
// the type of cgroup manager configured.
func SetCgroupManager(cgroupManager string) (CgroupManager, error) {
	switch cgroupManager {
	case systemdCgroupManager:
		return NewSystemdManager(), nil
	case cgroupfsCgroupManager:
		if node.CgroupIsV2() {
			return &CgroupfsManager{
				memoryPath:    CgroupMemoryPathV2,
				memoryMaxFile: cgroupMemoryMaxFileV2,
			}, nil
		}

		return &CgroupfsManager{
			memoryPath:    CgroupMemoryPathV1,
			memoryMaxFile: cgroupMemoryMaxFileV1,
			v1CtrCgMgr:    make(map[string]cgroups.Manager),
			v1SbCgMgr:     make(map[string]cgroups.Manager),
		}, nil
	default:
		return nil, fmt.Errorf("invalid cgroup manager: %s", cgroupManager)
	}
}

func verifyCgroupHasEnoughMemory(slicePath, memorySubsystemPath, memoryMaxFilename string, containerMinMemory int64) error {
	// read in the memory limit from memory max file
	fileData, err := os.ReadFile(filepath.Join(memorySubsystemPath, slicePath, memoryMaxFilename))
	if err != nil {
		if os.IsNotExist(err) {
			logrus.Warnf("Failed to find %s at path: %q", memoryMaxFilename, slicePath)

			return nil
		}

		return fmt.Errorf("unable to read memory file for cgroups at %s: %w", slicePath, err)
	}

	// strip off the newline character and convert it to an int
	strMemory := strings.TrimRight(string(fileData), "\n")
	if strMemory != "" && strMemory != "max" {
		memoryLimit, err := strconv.ParseInt(strMemory, 10, 64)
		if err != nil {
			return fmt.Errorf("error converting cgroup memory value from string to int %q: %w", strMemory, err)
		}
		// Compare with the minimum allowed memory limit
		if err := VerifyMemoryIsEnough(memoryLimit, containerMinMemory); err != nil {
			return fmt.Errorf("pod %w", err)
		}
	}

	return nil
}

// VerifyMemoryIsEnough verifies that the cgroup memory limit is above a specified minimum memory limit.
func VerifyMemoryIsEnough(memoryLimit, minMemory int64) error {
	if memoryLimit != 0 && memoryLimit < minMemory {
		return fmt.Errorf("set memory limit %d too low; should be at least %d bytes", memoryLimit, minMemory)
	}

	return nil
}

// MoveProcessToContainerCgroup moves process to the container cgroup.
func MoveProcessToContainerCgroup(containerPid, commandPid int) error {
	parentCgroupFile := fmt.Sprintf("/proc/%d/cgroup", containerPid)

	cgmap, err := cgroups.ParseCgroupFile(parentCgroupFile)
	if err != nil {
		return err
	}

	var dir string
	for controller, path := range cgmap {
		// For cgroups V2, controller will be an empty string
		dir = filepath.Join("/sys/fs/cgroup", controller, path)

		if cgroups.PathExists(dir) {
			if err := cgroups.WriteCgroupProc(dir, commandPid); err != nil {
				return err
			}
		}
	}

	return nil
}

// createSandboxCgroup takes the path of the sandbox parent and the desired containerCgroup
// It creates a cgroup through cgroupfs (as opposed to systemd) at the location cgroupRoot/sbParent/containerCgroup.
func createSandboxCgroup(sbParent, containerCgroup string) error {
	cg := &cgroups.Cgroup{
		Name:   containerCgroup,
		Parent: sbParent,
		Resources: &cgroups.Resources{
			SkipDevices: true,
		},
	}

	mgr, err := manager.New(cg)
	if err != nil {
		return err
	}

	// The reasoning for this code is slightly obscure. In situation where CPU load balancing is desired,
	// all cgroups must either have cpuset.sched_load_balance=0 or they should not have an intersecting cpuset
	// with the set that load balancing should be disabled on.
	// When this cgroup is created, it is easiest to set sched_load_balance to 0, especially because there will
	// not be any processes in this cgroup (or else we wouldn't need to call this).
	// Note: this should be done before Apply(-1) below, as Apply contains cpusetCopyIfNeeded(), which will
	// populate the cpuset with the parent's cpuset. However, it will be initialized to sched_load_balance=1
	// which will cause the kernel to move all cpusets out of their isolated sched_domain, causing unnecessary churn.
	if !node.CgroupIsV2() {
		path := mgr.Path("cpuset")
		if path == "" {
			return errors.New("failed to find cpuset for newly created cgroup")
		}

		if err := os.MkdirAll(path, 0o755); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to create cpuset for newly created cgroup: %w", err)
		}

		if err := cgroups.WriteFile(path, "cpuset.sched_load_balance", "0"); err != nil {
			return fmt.Errorf("failed to set sched_load_balance cpuset for newly created cgroup: %w", err)
		}
	}

	return mgr.Apply(-1)
}

func removeSandboxCgroup(sbParent, containerCgroup string) error {
	cg := &cgroups.Cgroup{
		Name:   containerCgroup,
		Parent: sbParent,
		Resources: &cgroups.Resources{
			SkipDevices: true,
		},
	}

	mgr, err := manager.New(cg)
	if err != nil {
		return err
	}

	return mgr.Destroy()
}

func containerCgroupPath(id string) string {
	return CrioPrefix + "-" + id
}

// LibctrManager creates a libcontainer cgroup manager for the given cgroup.
// The cgroup parameter is the name of the cgroup, parent is the parent path,
// and systemd indicates whether to use systemd cgroup driver.
func LibctrManager(cgroup, parent string, systemd bool) (cgroups.Manager, error) {
	if systemd {
		parent = filepath.Base(parent)
		if parent == "." {
			// libcontainer shorthand for root
			// see https://github.com/opencontainers/runc/blob/9fffadae8/libcontainer/cgroups/systemd/common.go#L71
			parent = "-.slice"
		}
	}

	cg := &cgroups.Cgroup{
		Name:   cgroup,
		Parent: parent,
		Resources: &cgroups.Resources{
			SkipDevices: true,
		},
		Systemd: systemd,
		// If the cgroup manager is systemd, then libcontainer
		// will construct the cgroup path (for scopes) as:
		// ScopePrefix-Name.scope. For slices, and for cgroupfs manager,
		// this will be ignored.
		// See: https://github.com/opencontainers/runc/tree/main/libcontainer/cgroups/systemd/common.go:getUnitName
		ScopePrefix: CrioPrefix,
	}

	return manager.New(cg)
}

// crunContainerCgroupManager returns the cgroup manager for the actual container cgroup.
// Some runtimes like crun create a sub-cgroup of the container to do the actual management,
// to enforce systemd's single owner rule. This function checks for and handles that case.
// If no sub-cgroup exists, it returns nil, nil.
func crunContainerCgroupManager(expectedContainerCgroup string) (cgroups.Manager, error) {
	// HACK: There isn't really a better way to check if the actual container cgroup is in a child cgroup of the expected.
	// We could check /proc/$pid/cgroup, but we need to be able to query this after the container exits and the process is gone.
	// We know the source of this: crun creates a sub cgroup of the container to do the actual management, to enforce systemd's single
	// owner rule. Thus, we need to hardcode this check.
	actualContainerCgroup := filepath.Join(expectedContainerCgroup, "container")
	// Choose cpuset as the cgroup to check, with little reason.
	cgroupRoot := CgroupMemoryPathV2
	if !node.CgroupIsV2() {
		cgroupRoot += "/cpuset"
	}

	// Normalize the path so that we don't add duplicate prefix.
	cgroupPath := filepath.Join(cgroupRoot, strings.TrimPrefix(actualContainerCgroup, cgroupRoot))
	if _, err := os.Stat(cgroupPath); err != nil {
		return nil, nil
	}
	// must be crun, make another LibctrManager. Regardless of cgroup driver, it will be treated as cgroupfs
	return LibctrManager(filepath.Base(actualContainerCgroup), filepath.Dir(actualContainerCgroup), false)
}

// execCgroupManager creates an exec cgroup for placing exec processes.
// containerCgroupAbsPath is the absolute path to the container's cgroup (without /sys/fs/cgroup prefix).
// Returns the cgroup manager for the exec cgroup.
//
// The exec cgroup location depends on whether crun created a "container" child cgroup:
//   - If crun's "container" child exists: exec cgroup is created under it
//   - Otherwise: exec cgroup is created directly under the container cgroup
func execCgroupManager(containerCgroupAbsPath string) (cgroups.Manager, error) {
	execCgroupParent := containerCgroupAbsPath

	// Check if crun created a "container" child cgroup
	if mgr, err := crunContainerCgroupManager(containerCgroupAbsPath); err == nil && mgr != nil {
		execCgroupParent = filepath.Join(containerCgroupAbsPath, "container")
	}

	return LibctrManager("exec", execCgroupParent, false)
}
