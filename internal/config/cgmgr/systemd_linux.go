//go:build linux

package cgmgr

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/containers/storage/pkg/unshare"
	systemdDbus "github.com/coreos/go-systemd/v22/dbus"
	"github.com/godbus/dbus/v5"
	"github.com/opencontainers/cgroups"
	"github.com/opencontainers/cgroups/systemd"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"

	"github.com/cri-o/cri-o/internal/config/node"
	"github.com/cri-o/cri-o/internal/dbusmgr"
	"github.com/cri-o/cri-o/utils"
)

const defaultSystemdParent = "system.slice"

// SystemdManager is the parent type of SystemdV{1,2}Manager.
// it defines all of the common functionality between V1 and V2.
type SystemdManager struct {
	memoryPath, memoryMaxFile string
	// a map of container ID to cgroup manager for cgroup v1
	// the reason we need this for v1 only is because the cost of creating a cgroup manager for v2 is very low
	// therefore, we don't need to cache it
	v1CtrCgMgr map[string]cgroups.Manager
	// a map of sandbox ID to cgroup manager for cgroup v1
	v1SbCgMgr map[string]cgroups.Manager
	dbusMgr   *dbusmgr.DbusConnManager
	mutex     sync.Mutex
}

func NewSystemdManager() *SystemdManager {
	systemdMgr := SystemdManager{}
	if node.CgroupIsV2() {
		systemdMgr.memoryPath = cgroupMemoryPathV2
		systemdMgr.memoryMaxFile = cgroupMemoryMaxFileV2
	} else {
		systemdMgr.memoryPath = cgroupMemoryPathV1
		systemdMgr.memoryMaxFile = cgroupMemoryMaxFileV1
		systemdMgr.v1CtrCgMgr = make(map[string]cgroups.Manager)
		systemdMgr.v1SbCgMgr = make(map[string]cgroups.Manager)
	}

	systemdMgr.dbusMgr = dbusmgr.NewDbusConnManager(unshare.IsRootless())

	return &systemdMgr
}

// Name returns the name of the cgroup manager (systemd).
func (*SystemdManager) Name() string {
	return systemdCgroupManager
}

// IsSystemd returns that it is a systemd cgroup manager.
func (*SystemdManager) IsSystemd() bool {
	return true
}

// ContainerCgroupPath takes arguments sandbox parent cgroup and container ID and returns
// the cgroup path for that containerID. If parentCgroup is empty, it
// uses the default parent system.slice.
func (*SystemdManager) ContainerCgroupPath(sbParent, containerID string) string {
	parent := defaultSystemdParent
	if sbParent != "" {
		parent = sbParent
	}

	return parent + ":" + CrioPrefix + ":" + containerID
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

// ContainerCgroupManager takes the cgroup parent, and container ID.
// It returns the raw libcontainer cgroup manager for that container.
func (m *SystemdManager) ContainerCgroupManager(sbParent, containerID string) (cgroups.Manager, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !node.CgroupIsV2() {
		if cgMgr, ok := m.v1CtrCgMgr[containerID]; ok {
			return cgMgr, nil
		}
	}

	cgPath, err := m.ContainerCgroupAbsolutePath(sbParent, containerID)
	if err != nil {
		return nil, err
	}
	// Due to a quirk of libcontainer's cgroup driver, cgroup name = containerID
	cgMgr, err := libctrManager(containerID, filepath.Dir(cgPath), true)
	if err != nil {
		return nil, err
	}

	if !node.CgroupIsV2() {
		// cache only cgroup v1 managers
		m.v1CtrCgMgr[containerID] = cgMgr
	}

	return cgMgr, nil
}

// ContainerCgroupStats takes the sandbox parent, and container ID.
// It creates a new cgroup if one does not already exist.
// It returns the cgroup stats for that container.
func (m *SystemdManager) ContainerCgroupStats(sbParent, containerID string) (*CgroupStats, error) {
	cgMgr, err := m.ContainerCgroupManager(sbParent, containerID)
	if err != nil {
		return nil, err
	}

	stats, err := cgMgr.GetStats()
	if err != nil {
		return nil, err
	}

	return libctrStatsToCgroupStats(stats), nil
}

// RemoveContainerCgManager removes the cgroup manager for the container.
func (m *SystemdManager) RemoveContainerCgManager(containerID string) {
	if !node.CgroupIsV2() {
		m.mutex.Lock()
		defer m.mutex.Unlock()

		delete(m.v1CtrCgMgr, containerID)
	}
}

// MoveConmonToCgroup takes the container ID, cgroup parent, conmon's cgroup (from the config) and conmon's PID
// It attempts to move conmon to the correct cgroup.
// cgroupPathToClean should always be returned empty. It is part of the interface to return the cgroup path
// that cri-o is responsible for cleaning up upon the container's death.
// Systemd takes care of this cleaning for us, so return an empty string.
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

		if resources.CPU.Quota != nil {
			props = append(props, systemdDbus.Property{
				Name:  "CPUQuota",
				Value: dbus.MakeVariant(resources.CPU.Quota),
			})
		}

		if resources.CPU.Period != nil {
			props = append(props, systemdDbus.Property{
				Name:  "CPUQuotaPeriodSec",
				Value: dbus.MakeVariant(resources.CPU.Period),
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

// SandboxCgroupPath takes the sandbox parent, sandbox ID, and container minimum memory.
// It returns the cgroup parent, cgroup path, and error.
// It also checks if enough memory is available in the given cgroup.
func (m *SystemdManager) SandboxCgroupPath(sbParent, sbID string, containerMinMemory int64) (cgParent, cgPath string, _ error) {
	if sbParent == "" {
		return "", "", nil
	}

	if !strings.HasSuffix(filepath.Base(sbParent), ".slice") {
		return "", "", fmt.Errorf("cri-o configured with systemd cgroup manager, but did not receive slice as parent: %s", sbParent)
	}

	cgParent = convertCgroupFsNameToSystemd(sbParent)

	if err := verifyCgroupHasEnoughMemory(sbParent, m.memoryPath, m.memoryMaxFile, containerMinMemory); err != nil {
		return "", "", err
	}

	cgPath = cgParent + ":" + CrioPrefix + ":" + sbID

	return cgParent, cgPath, nil
}

// SandboxCgroupManager takes the cgroup parent, and sandbox ID.
// It returns the raw libcontainer cgroup manager for that sandbox.
func (m *SystemdManager) SandboxCgroupManager(sbParent, sbID string) (cgroups.Manager, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !node.CgroupIsV2() {
		if cgMgr, ok := m.v1SbCgMgr[sbID]; ok {
			return cgMgr, nil
		}
	}

	_, cgPath, err := sandboxCgroupAbsolutePath(sbParent)
	if err != nil {
		return nil, err
	}

	cgMgr, err := libctrManager(filepath.Base(cgPath), filepath.Dir(cgPath), true)
	if err != nil {
		return nil, err
	}

	if !node.CgroupIsV2() {
		// cache only cgroup v1 managers
		m.v1SbCgMgr[sbID] = cgMgr
	}

	return cgMgr, nil
}

// SandboxCgroupStats takes the sandbox parent, and sandbox ID.
// It creates a new cgroup for that sandbox if it does not already exist.
// It returns the cgroup stats for that sandbox.
func (m *SystemdManager) SandboxCgroupStats(sbParent, sbID string) (*CgroupStats, error) {
	cgMgr, err := m.SandboxCgroupManager(sbParent, sbID)
	if err != nil {
		return nil, err
	}

	stats, err := cgMgr.GetStats()
	if err != nil {
		return nil, err
	}

	return libctrStatsToCgroupStats(stats), nil
}

// RemoveSandboxCgroupManager removes cgroup manager for the sandbox.
func (m *SystemdManager) RemoveSandboxCgManager(sbID string) {
	if !node.CgroupIsV2() {
		m.mutex.Lock()
		defer m.mutex.Unlock()

		delete(m.v1SbCgMgr, sbID)
	}
}

//nolint:unparam // golangci-lint claims cgParent is unused, though it's being used to include documentation inline.
func sandboxCgroupAbsolutePath(sbParent string) (cgParent, slicePath string, err error) {
	cgParent = convertCgroupFsNameToSystemd(sbParent)

	slicePath, err = systemd.ExpandSlice(cgParent)
	if err != nil {
		return "", "", fmt.Errorf("expanding systemd slice path for %q: %w", cgParent, err)
	}

	return cgParent, slicePath, nil
}

// convertCgroupFsNameToSystemd converts an expanded cgroupfs name to its systemd name.
// For example, it will convert test.slice/test-a.slice/test-a-b.slice to become test-a-b.slice.
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
