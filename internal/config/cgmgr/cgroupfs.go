//go:build linux
// +build linux

package cgmgr

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/containers/podman/v3/pkg/cgroups"
	"github.com/cri-o/cri-o/internal/config/node"
	"github.com/cri-o/cri-o/utils"
	libctr "github.com/opencontainers/runc/libcontainer/cgroups"
	"github.com/opencontainers/runc/libcontainer/cgroups/fs"
	"github.com/opencontainers/runc/libcontainer/cgroups/fs2"
	cgcfgs "github.com/opencontainers/runc/libcontainer/configs"
	"github.com/opencontainers/runc/libcontainer/devices"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// CgroupfsManager defines functionality whrn **** TODO: Update this
type CgroupfsManager struct {
	memoryPath, memoryMaxFile string
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
	return filepath.Join("/", parent, crioPrefix+"-"+containerID)
}

// PopulateContainerCgroupStats takes arguments sandbox parent cgroup, container ID, and
// containers stats object. It fills the object with information from the cgroup found
// given that parent and ID
func (m *CgroupfsManager) PopulateContainerCgroupStats(sbParent, containerID string, stats *types.ContainerStats) error {
	cgPath, err := m.ContainerCgroupAbsolutePath(sbParent, containerID)
	if err != nil {
		return err
	}
	return populateContainerCgroupStatsFromPath(cgPath, stats)
}

// ContainerCgroupAbsolutePath just calls ContainerCgroupPath,
// because they both return the absolute path
func (m *CgroupfsManager) ContainerCgroupAbsolutePath(sbParent, containerID string) (string, error) {
	return m.ContainerCgroupPath(sbParent, containerID), nil
}

// SandboxCgroupPath takes the sandbox parent, and sandbox ID. It
// returns the cgroup parent, cgroup path, and error.
func (m *CgroupfsManager) SandboxCgroupPath(sbParent, sbID string) (cgParent, cgPath string, _ error) {
	if strings.HasSuffix(path.Base(sbParent), ".slice") {
		return "", "", fmt.Errorf("cri-o configured with cgroupfs cgroup manager, but received systemd slice as parent: %s", sbParent)
	}

	if err := verifyCgroupHasEnoughMemory(sbParent, m.memoryPath, m.memoryMaxFile); err != nil {
		return "", "", err
	}

	return sbParent, filepath.Join(sbParent, crioPrefix+"-"+sbID), nil
}

// PopulateSandboxCgroupStats takes arguments sandbox parent cgroup and sandbox stats object
// It fills the object with information from the cgroup found given that cgroup
func (m *CgroupfsManager) PopulateSandboxCgroupStats(sbParent string, stats *types.PodSandboxStats) error {
	_, cgPath, err := sandboxCgroupAbsolutePath(sbParent)
	if err != nil {
		return err
	}
	return populateSandboxCgroupStatsFromPath(cgPath, stats)
}

// MoveConmonToCgroup takes the container ID, cgroup parent, conmon's cgroup (from the config) and conmon's PID
// It attempts to move conmon to the correct cgroup.
// It returns the cgroupfs parent that conmon was put into
// so that CRI-O can clean the cgroup path of the newly added conmon once the process terminates (systemd handles this for us)
func (*CgroupfsManager) MoveConmonToCgroup(cid, cgroupParent, conmonCgroup string, pid int, resources *rspec.LinuxResources) (cgroupPathToClean string, _ error) {
	if conmonCgroup != utils.PodCgroupName && conmonCgroup != "" {
		return "", errors.Errorf("conmon cgroup %s invalid for cgroupfs", conmonCgroup)
	}

	if resources == nil {
		resources = &rspec.LinuxResources{}
	}

	cgroupPath := fmt.Sprintf("%s/crio-conmon-%s", cgroupParent, cid)
	control, err := cgroups.New(cgroupPath, &rspec.LinuxResources{})
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
		return "", errors.Wrapf(err, "Failed to add conmon to cgroupfs sandbox cgroup")
	}
	return cgroupPath, nil
}

func setWorkloadSettings(cgPath string, resources *rspec.LinuxResources) error {
	var mgr libctr.Manager
	if resources.CPU == nil {
		return nil
	}

	paths := map[string]string{
		"cpuset":  filepath.Join("/sys/fs/cgroup", "cpuset", cgPath),
		"cpu":     filepath.Join("/sys/fs/cgroup", "cpu", cgPath),
		"freezer": filepath.Join("/sys/fs/cgroup", "freezer", cgPath),
		"devices": filepath.Join("/sys/fs/cgroup", "devices", cgPath),
	}

	cg := &cgcfgs.Cgroup{
		Name:      cgPath,
		Resources: &cgcfgs.Resources{},
	}
	if resources.CPU.Cpus != "" {
		cg.Resources.CpusetCpus = resources.CPU.Cpus
	}
	if resources.CPU.Shares != nil {
		cg.Resources.CpuShares = *resources.CPU.Shares
	}

	// We need to white list all devices
	// so containers created underneath won't fail
	cg.Resources.Devices = []*devices.Rule{
		{
			Type:  devices.WildcardDevice,
			Allow: true,
		},
	}

	var err error
	if node.CgroupIsV2() {
		mgr, err = fs2.NewManager(cg, cgPath)

	} else {
		mgr, err = fs.NewManager(cg, paths)
	}
	if err != nil {
		return err
	}
	return mgr.Set(cg.Resources)
}

// createSandboxCgroup takes the sandbox parent, and sandbox ID.
// It creates a new cgroup for that sandbox, which is useful when spoofing an infra container.
func createSandboxCgroup(sbParent, containerID string, mgr CgroupManager) error {
	cgroupAbsolutePath, err := mgr.ContainerCgroupAbsolutePath(sbParent, containerID)
	if err != nil {
		return err
	}
	_, err = cgroups.New(cgroupAbsolutePath, &rspec.LinuxResources{})
	return err
}

// CreateSandboxCgroup calls the helper function createSandboxCgroup for this manager.
func (m *CgroupfsManager) CreateSandboxCgroup(sbParent, containerID string) error {
	return createSandboxCgroup(sbParent, containerID, m)
}
