//go:build linux
// +build linux

package cgmgr

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cri-o/cri-o/internal/config/node"
	libctr "github.com/opencontainers/runc/libcontainer/cgroups"
	libctrCgMgr "github.com/opencontainers/runc/libcontainer/cgroups/manager"
	cgcfgs "github.com/opencontainers/runc/libcontainer/configs"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

const (
	CrioPrefix = "crio"
	// minMemoryLimit is the minimum memory that must be set for a container.
	// A lower value would result in the container failing to start.
	// this value has been arrived at for runc on x86_64 hardware
	minMemoryLimit = 12 * 1024 * 1024
	// CgroupfsCgroupManager represents cgroupfs native cgroup manager
	cgroupfsCgroupManager = "cgroupfs"
	// SystemdCgroupManager represents systemd native cgroup manager
	systemdCgroupManager = "systemd"

	DefaultCgroupManager = systemdCgroupManager

	// these constants define the path and name of the memory max file
	// for v1 and v2 respectively
	cgroupMemoryPathV1    = "/sys/fs/cgroup/memory"
	cgroupMemoryMaxFileV1 = "memory.limit_in_bytes"
	cgroupMemoryPathV2    = "/sys/fs/cgroup"
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
	// PopulateContainerCgroupStats fills the stats object with information from the cgroup found
	// given a cgroup parent and container ID.
	PopulateContainerCgroupStats(sbParent, containerID string, stats *types.ContainerStats) error
	// ContainerCgroupManager takes the cgroup parent, and container ID.
	// It returns the raw libcontainer cgroup manager for that container.
	ContainerCgroupManager(sbParent, containerID string) (libctr.Manager, error)
	// RemoveContainerCgManager removes the cgroup manager for the container
	RemoveContainerCgManager(containerID string)
	// SandboxCgroupPath takes the sandbox parent, and sandbox ID. It
	// returns the cgroup parent, cgroup path, and error. For systemd cgroups,
	// it also checks there is enough memory in the given cgroup
	SandboxCgroupPath(string, string) (string, string, error)
	// PopulateContainerCgroupStats takes arguments sandbox parent cgroup, and sandbox stats object.
	// It fills the object with information from the cgroup found given that parent.
	PopulateSandboxCgroupStats(sbParent string, stats *types.PodSandboxStats) error
	// SandboxCgroupManager takes the cgroup parent, and sandbox ID.
	// It returns the raw libcontainer cgroup manager for that sandbox.
	SandboxCgroupManager(sbParent, sbID string) (libctr.Manager, error)
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
}

// New creates a new CgroupManager with defaults
func New() CgroupManager {
	cm, err := SetCgroupManager(DefaultCgroupManager)
	if err != nil {
		panic(err)
	}
	return cm
}

// SetCgroupManager takes a string and branches on it to return
// the type of cgroup manager configured
func SetCgroupManager(cgroupManager string) (CgroupManager, error) {
	switch cgroupManager {
	case systemdCgroupManager:
		return NewSystemdManager(), nil
	case cgroupfsCgroupManager:
		if node.CgroupIsV2() {
			return &CgroupfsManager{
				memoryPath:    cgroupMemoryPathV2,
				memoryMaxFile: cgroupMemoryMaxFileV2,
			}, nil
		}
		return &CgroupfsManager{
			memoryPath:    cgroupMemoryPathV1,
			memoryMaxFile: cgroupMemoryMaxFileV1,
			v1CtrCgMgr:    make(map[string]libctr.Manager),
			v1SbCgMgr:     make(map[string]libctr.Manager),
		}, nil
	default:
		return nil, fmt.Errorf("invalid cgroup manager: %s", cgroupManager)
	}
}

func verifyCgroupHasEnoughMemory(slicePath, memorySubsystemPath, memoryMaxFilename string) error {
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
		if err := VerifyMemoryIsEnough(memoryLimit); err != nil {
			return fmt.Errorf("pod %w", err)
		}
	}
	return nil
}

// VerifyMemoryIsEnough verifies that the cgroup memory limit is above a specified minimum memory limit.
func VerifyMemoryIsEnough(memoryLimit int64) error {
	if memoryLimit != 0 && memoryLimit < minMemoryLimit {
		return fmt.Errorf("set memory limit %d too low; should be at least %d", memoryLimit, minMemoryLimit)
	}
	return nil
}

// MoveProcessToContainerCgroup moves process to the container cgroup
func MoveProcessToContainerCgroup(containerPid, commandPid int) error {
	parentCgroupFile := fmt.Sprintf("/proc/%d/cgroup", containerPid)
	cgmap, err := libctr.ParseCgroupFile(parentCgroupFile)
	if err != nil {
		return err
	}

	var dir string
	for controller, path := range cgmap {
		// For cgroups V2, controller will be an empty string
		dir = filepath.Join("/sys/fs/cgroup", controller, path)

		if libctr.PathExists(dir) {
			if err := libctr.WriteCgroupProc(dir, commandPid); err != nil {
				return err
			}
		}
	}
	return nil
}

// createSandboxCgroup takes the path of the sandbox parent and the desired containerCgroup
// It creates a cgroup through cgroupfs (as opposed to systemd) at the location cgroupRoot/sbParent/containerCgroup.
func createSandboxCgroup(sbParent, containerCgroup string) error {
	cg := &cgcfgs.Cgroup{
		Name:   containerCgroup,
		Parent: sbParent,
		Resources: &cgcfgs.Resources{
			SkipDevices: true,
		},
	}
	mgr, err := libctrCgMgr.New(cg)
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
			return fmt.Errorf("failed to find cpuset for newly created cgroup")
		}
		if err := os.MkdirAll(path, 0o755); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to create cpuset for newly created cgroup: %w", err)
		}
		if err := libctr.WriteFile(path, "cpuset.sched_load_balance", "0"); err != nil {
			return fmt.Errorf("failed to set sched_load_balance cpuset for newly created cgroup: %w", err)
		}
	}

	return mgr.Apply(-1)
}

func removeSandboxCgroup(sbParent, containerCgroup string) error {
	cg := &cgcfgs.Cgroup{
		Name:   containerCgroup,
		Parent: sbParent,
		Resources: &cgcfgs.Resources{
			SkipDevices: true,
		},
	}
	mgr, err := libctrCgMgr.New(cg)
	if err != nil {
		return err
	}

	return mgr.Destroy()
}

func containerCgroupPath(id string) string {
	return CrioPrefix + "-" + id
}
