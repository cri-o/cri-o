//go:build linux
// +build linux

package cgmgr

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/containers/common/pkg/cgroups"
	"github.com/containers/storage/pkg/unshare"
	"github.com/cri-o/cri-o/internal/config/node"
	"github.com/cri-o/cri-o/utils"
	libctrCg "github.com/opencontainers/runc/libcontainer/cgroups"
	libctrCgMgr "github.com/opencontainers/runc/libcontainer/cgroups/manager"
	cgcfgs "github.com/opencontainers/runc/libcontainer/configs"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

// CgroupfsManager defines functionality whrn **** TODO: Update this
type CgroupfsManager struct {
	memoryPath, memoryMaxFile string
	// a map of container ID to cgroup manager for cgroup v1
	// the reason we need this for v1 only is because the cost of creating a cgroup manager for v2 is very low
	// and we don't need to cache it
	v1CtrCgMgr map[string]libctrCg.Manager
	// a map of sandbox ID to cgroup manager for cgroup v1
	v1SbCgMgr map[string]libctrCg.Manager
	mutex     sync.Mutex
}

const (
	defaultCgroupfsParent = "/crio"
)

// Name returns the name of the cgroup manager (cgroupfs)
func (*CgroupfsManager) Name() string {
	return cgroupfsCgroupManager
}

// IsSystemd returns that this is not a systemd cgroup manager
func (*CgroupfsManager) IsSystemd() bool {
	return false
}

// ContainerCgroupPath takes arguments sandbox parent cgroup and container ID and returns
// the cgroup path for that containerID. If parentCgroup is empty, it
// uses the default parent /crio
func (*CgroupfsManager) ContainerCgroupPath(sbParent, containerID string) string {
	parent := defaultCgroupfsParent
	if sbParent != "" {
		parent = sbParent
	}
	return filepath.Join("/", parent, containerCgroupPath(containerID))
}

// ContainerCgroupAbsolutePath just calls ContainerCgroupPath,
// because they both return the absolute path
func (m *CgroupfsManager) ContainerCgroupAbsolutePath(sbParent, containerID string) (string, error) {
	return m.ContainerCgroupPath(sbParent, containerID), nil
}

// ContainerCgroupManager takes the cgroup parent, and container ID.
// It returns the raw libcontainer cgroup manager for that container.
func (m *CgroupfsManager) ContainerCgroupManager(sbParent, containerID string) (libctrCg.Manager, error) {
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
	cgMgr, err := libctrManager(filepath.Base(cgPath), filepath.Dir(cgPath), false)
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
func (m *CgroupfsManager) ContainerCgroupStats(sbParent, containerID string) (*CgroupStats, error) {
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

// RemoveContainerCgManager removes the cgroup manager for the container
func (m *CgroupfsManager) RemoveContainerCgManager(containerID string) {
	if !node.CgroupIsV2() {
		m.mutex.Lock()
		defer m.mutex.Unlock()
		delete(m.v1CtrCgMgr, containerID)
	}
}

// SandboxCgroupPath takes the sandbox parent, sandbox ID, and container minimum memory.
// It returns the cgroup parent, cgroup path, and error.
// It also checks if enough memory is available in the given cgroup.
func (m *CgroupfsManager) SandboxCgroupPath(sbParent, sbID string, containerMinMemory int64) (cgParent, cgPath string, _ error) {
	if strings.HasSuffix(path.Base(sbParent), ".slice") {
		return "", "", fmt.Errorf("cri-o configured with cgroupfs cgroup manager, but received systemd slice as parent: %s", sbParent)
	}

	if err := verifyCgroupHasEnoughMemory(sbParent, m.memoryPath, m.memoryMaxFile, containerMinMemory); err != nil {
		return "", "", err
	}

	return sbParent, filepath.Join(sbParent, containerCgroupPath(sbID)), nil
}

// SandboxCgroupManager takes the cgroup parent, and sandbox ID.
// It returns the raw libcontainer cgroup manager for that sandbox.
func (m *CgroupfsManager) SandboxCgroupManager(sbParent, sbID string) (libctrCg.Manager, error) {
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
	cgMgr, err := libctrManager(filepath.Base(cgPath), filepath.Dir(cgPath), false)
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
func (m *CgroupfsManager) SandboxCgroupStats(sbParent, sbID string) (*CgroupStats, error) {
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

// RemoveSandboxCgroupManager removes the cgroup manager for the sandbox
func (m *CgroupfsManager) RemoveSandboxCgManager(sbID string) {
	if !node.CgroupIsV2() {
		m.mutex.Lock()
		defer m.mutex.Unlock()
		delete(m.v1SbCgMgr, sbID)
	}
}

// MoveConmonToCgroup takes the container ID, cgroup parent, conmon's cgroup (from the config) and conmon's PID
// It attempts to move conmon to the correct cgroup.
// It returns the cgroupfs parent that conmon was put into
// so that CRI-O can clean the cgroup path of the newly added conmon once the process terminates (systemd handles this for us)
func (*CgroupfsManager) MoveConmonToCgroup(cid, cgroupParent, conmonCgroup string, pid int, resources *rspec.LinuxResources) (cgroupPathToClean string, _ error) {
	if conmonCgroup != utils.PodCgroupName && conmonCgroup != "" {
		return "", fmt.Errorf("conmon cgroup %s invalid for cgroupfs", conmonCgroup)
	}

	if resources == nil {
		resources = &rspec.LinuxResources{}
	}

	cgroupPath := fmt.Sprintf("%s/crio-conmon-%s", cgroupParent, cid)
	control, err := cgroups.New(cgroupPath, &cgcfgs.Resources{})
	if err != nil {
		logrus.Warnf("Failed to add conmon to cgroupfs sandbox cgroup: %v", err)
	}
	if control == nil {
		return cgroupPath, nil
	}

	if err := setWorkloadSettings(cgroupPath, resources); err != nil {
		return cgroupPath, err
	}

	// Record conmon's cgroup path in the container, so we can properly
	// clean it up when removing the container.
	// Here we should defer a crio-connmon- cgroup hierarchy deletion, but it will
	// always fail as conmon's pid is still there.
	// Fortunately, kubelet takes care of deleting this for us, so the leak will
	// only happens in corner case where one does a manual deletion of the container
	// through e.g. runc. This should be handled by implementing a conmon monitoring
	// routine that does the cgroup cleanup once conmon is terminated.
	if err := control.AddPid(pid); err != nil {
		return "", fmt.Errorf("failed to add conmon to cgroupfs sandbox cgroup: %w", err)
	}
	return cgroupPath, nil
}

func setWorkloadSettings(cgPath string, resources *rspec.LinuxResources) (err error) {
	if resources.CPU == nil {
		return nil
	}

	cg := &cgcfgs.Cgroup{
		Path: "/" + cgPath,
		Resources: &cgcfgs.Resources{
			SkipDevices: true,
			CpusetCpus:  resources.CPU.Cpus,
		},
		Rootless: unshare.IsRootless(),
	}
	if resources.CPU.Shares != nil {
		cg.Resources.CpuShares = *resources.CPU.Shares
	}
	if resources.CPU.Quota != nil {
		cg.Resources.CpuQuota = *resources.CPU.Quota
	}
	if resources.CPU.Period != nil {
		cg.Resources.CpuPeriod = *resources.CPU.Period
	}

	mgr, err := libctrCgMgr.New(cg)
	if err != nil {
		return err
	}

	return mgr.Set(cg.Resources)
}

// CreateSandboxCgroup calls the helper function createSandboxCgroup for this manager.
func (m *CgroupfsManager) CreateSandboxCgroup(sbParent, containerID string) error {
	// prepend "/" to sbParent so the fs driver interprets it as an absolute path
	// and the cgroup isn't created as a relative path to the cgroups of the CRI-O process.
	// https://github.com/opencontainers/runc/blob/fd5debf3aa/libcontainer/cgroups/fs/paths.go#L156
	return createSandboxCgroup(filepath.Join("/", sbParent), containerCgroupPath(containerID))
}

// RemoveSandboxCgroup calls the helper function removeSandboxCgroup for this manager.
func (m *CgroupfsManager) RemoveSandboxCgroup(sbParent, containerID string) error {
	// prepend "/" to sbParent so the fs driver interprets it as an absolute path
	// and the cgroup isn't created as a relative path to the cgroups of the CRI-O process.
	// https://github.com/opencontainers/runc/blob/fd5debf3aa/libcontainer/cgroups/fs/paths.go#L156
	return removeSandboxCgroup(filepath.Join("/", sbParent), containerCgroupPath(containerID))
}
