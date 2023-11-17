//go:build linux
// +build linux

package cgmgr

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/containers/podman/v4/pkg/rootless"
	systemdDbus "github.com/coreos/go-systemd/v22/dbus"
	"github.com/cri-o/cri-o/internal/config/node"
	"github.com/cri-o/cri-o/internal/dbusmgr"
	"github.com/cri-o/cri-o/utils"
	"github.com/godbus/dbus/v5"
	"github.com/opencontainers/runc/libcontainer/cgroups"
	libctrCgMgr "github.com/opencontainers/runc/libcontainer/cgroups/manager"
	"github.com/opencontainers/runc/libcontainer/cgroups/systemd"
	cgcfgs "github.com/opencontainers/runc/libcontainer/configs"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

const defaultSystemdParent = "system.slice"

// SystemdManager is the parent type of SystemdV{1,2}Manager.
// it defines all of the common functionality between V1 and V2
type SystemdManager struct {
	memoryPath, memoryMaxFile string
	cgManagers                map[string]cgroups.Manager
	dbusMgr                   *dbusmgr.DbusConnManager
}

func NewSystemdManager() *SystemdManager {
	systemdMgr := SystemdManager{
		memoryPath:    cgroupMemoryPathV1,
		cgManagers:    make(map[string]cgroups.Manager),
		memoryMaxFile: cgroupMemoryMaxFileV1,
	}
	if node.CgroupIsV2() {
		systemdMgr.memoryPath = cgroupMemoryPathV2
		systemdMgr.memoryMaxFile = cgroupMemoryMaxFileV2
	}
	systemdMgr.dbusMgr = dbusmgr.NewDbusConnManager(rootless.IsRootless())

	return &systemdMgr
}

// Name returns the name of the cgroup manager (systemd)
func (*SystemdManager) Name() string {
	return systemdCgroupManager
}

// IsSystemd returns that it is a systemd cgroup manager
func (*SystemdManager) IsSystemd() bool {
	return true
}

// ContainerCgroupPath takes arguments sandbox parent cgroup and container ID and returns
// the cgroup path for that containerID. If parentCgroup is empty, it
// uses the default parent system.slice
func (*SystemdManager) ContainerCgroupPath(sbParent, containerID string) string {
	parent := defaultSystemdParent
	if sbParent != "" {
		parent = sbParent
	}
	return parent + ":" + CrioPrefix + ":" + containerID
}

// ContainerCgroupStats returns a stats object with information from the cgroup found
// given a cgroup parent and container ID.
func (m *SystemdManager) ContainerCgroupStats(sbParent, containerID string) (*CgroupStats, error) {
	cgPath, err := m.ContainerCgroupAbsolutePath(sbParent, containerID)
	if err != nil {
		return nil, err
	}
	cgMgr, err := m.getOrCreateCtrCgManager(cgPath, containerID)
	if err != nil {
		return nil, err
	}
	libCtrStats, err := cgMgr.GetStats()
	if err != nil {
		return nil, err
	}
	cgstats := &CgroupStats{
		MostStats:     libCtrStats,
		OtherMemStats: generateOtherMemoryStats(libCtrStats.MemoryStats),
		SystemNano:    time.Now().UnixNano(),
	}
	return cgstats, nil
}

func (m *SystemdManager) getOrCreateCtrCgManager(cgPath, containerID string) (cgroups.Manager, error) {
	if cgMgr, ok := m.cgManagers[containerID]; ok {
		return cgMgr, nil
	}
	ctrName, parentCgroup := filepath.Base(cgPath), filepath.Dir(cgPath)
	// TODO: Add relevant config options
	cg := &cgcfgs.Cgroup{
		Name:   ctrName,
		Parent: parentCgroup,
		// The Systemd flag should be set to true. However, creating a cgroup manager will if this flag is set to true. (Possibly a Bug)
		//Systemd: true,
		Resources: &cgcfgs.Resources{
			SkipDevices: true,
		},
	}
	cgMgr, err := libctrCgMgr.New(cg)
	if err != nil {
		logrus.Errorf("Failed to create cgroup manager for container %s: %v", containerID, err)
		return nil, err
	}
	m.cgManagers[containerID] = cgMgr
	return cgMgr, nil

}

// GetCtrCgroupManager takes the cgroup parent, and container ID.
// It returns the raw libcontainer cgroup manager for that container.
func (m *SystemdManager) GetCtrCgroupManager(sbParent, containerID string) (cgroups.Manager, error) {
	cgPath, err := m.ContainerCgroupAbsolutePath(sbParent, containerID)
	if err != nil {
		return nil, err
	}
	return m.getOrCreateCtrCgManager(cgPath, containerID)
}

func (m *SystemdManager) RemoveCtrCgManager(containerID string) {
	if cgMgr, ok := m.cgManagers[containerID]; ok {
		if err := cgMgr.Destroy(); err != nil {
			logrus.Errorf("Failed to destroy cgroup manager for container %s: %v", containerID, err)
		}
		delete(m.cgManagers, containerID)
	}
}

func (m *SystemdManager) ContainerCgroupAbsolutePath(sbParent, containerID string) (string, error) {
	parent := defaultSystemdParent
	if sbParent != "" {
		parent = sbParent
	}
	logrus.Debugf("Expanding systemd cgroup slice %v", parent)
	cgroup, err := systemd.ExpandSlice(parent)
	if err != nil {
		return "", fmt.Errorf("expanding systemd slice to get container %s stats: %w", containerID, err)
	}

	return filepath.Join(cgroup, containerCgroupPath(containerID)+".scope"), nil
}

// MoveConmonToCgroup takes the container ID, cgroup parent, conmon's cgroup (from the config) and conmon's PID
// It attempts to move conmon to the correct cgroup.
// cgroupPathToClean should always be returned empty. It is part of the interface to return the cgroup path
// that cri-o is responsible for cleaning up upon the container's death.
// Systemd takes care of this cleaning for us, so return an empty string
func (m *SystemdManager) MoveConmonToCgroup(cid, cgroupParent, conmonCgroup string, pid int, resources *rspec.LinuxResources) (cgroupPathToClean string, _ error) {
	if strings.HasSuffix(conmonCgroup, ".slice") {
		cgroupParent = conmonCgroup
	}
	conmonUnitName := fmt.Sprintf("crio-conmon-%s.scope", cid)

	// Set the systemd KillSignal to SIGPIPE that conmon ignores.
	// This helps during node shutdown so that conmon waits for the container
	// to exit and doesn't forward the SIGTERM that it gets.
	props := []systemdDbus.Property{
		{
			Name:  "KillSignal",
			Value: dbus.MakeVariant(int(unix.SIGPIPE)),
		},
		systemdDbus.PropAfter("crio.service"),
	}

	if resources != nil && resources.CPU != nil {
		if resources.CPU.Cpus != "" {
			if !node.SystemdHasAllowedCPUs() {
				logrus.Errorf("Systemd does not support AllowedCPUs; skipping setting for workload")
			} else {
				bits, err := systemd.RangeToBits(resources.CPU.Cpus)
				if err != nil {
					return "", fmt.Errorf("cpuset conversion error: %w", err)
				}
				props = append(props, systemdDbus.Property{
					Name:  "AllowedCPUs",
					Value: dbus.MakeVariant(bits),
				})
			}
		}
		if resources.CPU.Shares != nil {
			props = append(props, systemdDbus.Property{
				Name:  "CPUShares",
				Value: dbus.MakeVariant(resources.CPU.Shares),
			})
		}
	}

	logrus.Debugf("Running conmon under slice %s and unitName %s", cgroupParent, conmonUnitName)
	if err := utils.RunUnderSystemdScope(m.dbusMgr, pid, cgroupParent, conmonUnitName, props...); err != nil {
		return "", fmt.Errorf("failed to add conmon to systemd sandbox cgroup: %w", err)
	}
	// return empty string as path because cgroup cleanup is done by systemd
	return "", nil
}

// SandboxCgroupPath takes the sandbox parent, and sandbox ID. It
// returns the cgroup parent, cgroup path, and error.
// It also checks there is enough memory in the given cgroup
func (m *SystemdManager) SandboxCgroupPath(sbParent, sbID string) (cgParent, cgPath string, _ error) {
	if sbParent == "" {
		return "", "", nil
	}

	if !strings.HasSuffix(filepath.Base(sbParent), ".slice") {
		return "", "", fmt.Errorf("cri-o configured with systemd cgroup manager, but did not receive slice as parent: %s", sbParent)
	}

	cgParent = convertCgroupFsNameToSystemd(sbParent)
	if err := verifyCgroupHasEnoughMemory(sbParent, m.memoryPath, m.memoryMaxFile); err != nil {
		return "", "", err
	}

	cgPath = cgParent + ":" + CrioPrefix + ":" + sbID

	return cgParent, cgPath, nil
}

// SandboxCgroupStats takes arguments sandbox parent cgroup, and sandbox stats object.
// It returns an object with information from the cgroup found given that parent.
func (m *SystemdManager) SandboxCgroupStats(sbParent string) (*CgroupStats, error) {
	_, cgPath, err := sandboxCgroupAbsolutePath(sbParent)
	if err != nil {
		return nil, err
	}
	return cgroupStatsFromPath(cgPath)
}

// nolint: unparam // golangci-lint claims cgParent is unused, though it's being used to include documentation inline.
func sandboxCgroupAbsolutePath(sbParent string) (cgParent, slicePath string, err error) {
	cgParent = convertCgroupFsNameToSystemd(sbParent)
	slicePath, err = systemd.ExpandSlice(cgParent)
	if err != nil {
		return "", "", fmt.Errorf("expanding systemd slice path for %q: %w", cgParent, err)
	}
	return cgParent, slicePath, nil
}

// convertCgroupFsNameToSystemd converts an expanded cgroupfs name to its systemd name.
// For example, it will convert test.slice/test-a.slice/test-a-b.slice to become test-a-b.slice
func convertCgroupFsNameToSystemd(cgroupfsName string) string {
	// TODO: see if libcontainer systemd implementation could use something similar, and if so, move
	// this function up to that library.  At that time, it would most likely do validation specific to systemd
	// above and beyond the simple assumption here that the base of the path encodes the hierarchy
	// per systemd convention.
	return path.Base(cgroupfsName)
}

// CreateSandboxCgroup calls the helper function createSandboxCgroup for this manager.
// Note: createSandboxCgroup will create a cgroupfs cgroup for the infra container underneath the pod slice.
// It will not use dbus to create this cgroup, but instead call libcontainer's cgroupfs manager directly.
// This is because a scope created here will not have a process within it (as it's usually for a dropped infra container),
// and a slice cannot have the required `crio` prefix (while still being within the pod slice).
// Ultimately, this cgroup is required for cAdvisor to be able to register the pod and collect network metrics for it.
// This work will not be relevant when CRI-O is responsible for gathering pod metrics (KEP-2371), but is required until that's done.
func (m *SystemdManager) CreateSandboxCgroup(sbParent, containerID string) error {
	// sbParent should always be specified by kubelet, but sometimes not by critest/crictl.
	// Skip creation in this case.
	if sbParent == "" {
		logrus.Infof("Not creating sandbox cgroup: sbParent is empty")
		return nil
	}
	expandedParent, err := systemd.ExpandSlice(sbParent)
	if err != nil {
		return err
	}
	return createSandboxCgroup(expandedParent, containerCgroupPath(containerID))
}

// RemoveSandboxCgroup calls the helper function removeSandboxCgroup for this manager.
func (m *SystemdManager) RemoveSandboxCgroup(sbParent, containerID string) error {
	// sbParent should always be specified by kubelet, but sometimes not by critest/crictl.
	// Skip creation in this case.
	if sbParent == "" {
		logrus.Infof("Not creating sandbox cgroup: sbParent is empty")
		return nil
	}
	expandedParent, err := systemd.ExpandSlice(sbParent)
	if err != nil {
		return err
	}
	return removeSandboxCgroup(expandedParent, containerCgroupPath(containerID))
}
