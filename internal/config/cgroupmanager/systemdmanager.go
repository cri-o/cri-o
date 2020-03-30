// +build linux

package cgroupmanager

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cri-o/cri-o/utils"
	"github.com/opencontainers/runc/libcontainer/cgroups/systemd"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const defaultSystemdParent = "system.slice"

type Systemdv1Manager struct{}

// Name returns the name of the cgroup manager (systemd)
func (*Systemdv1Manager) Name() string {
	return SystemdCgroupManager
}

// IsSystemd returns that it is a systemd cgroup manager
func (*Systemdv1Manager) IsSystemd() bool {
	return true
}

// GetContainerCgroupPath takes arguments sandbox parent cgroup and container ID and returns
// the cgroup path for that containerID. If parentCgroup is empty, it
// uses the default parent system.slice
func (*Systemdv1Manager) GetContainerCgroupPath(sbParent, containerID string) string {
	return getContainerCgroupPath(sbParent, containerID)
}

// GetSandboxCgroupPath takes the sandbox parent, and sandbox ID. It
// returns the cgroup parent, cgroup path, and error.
// it also checks there is enough memory in the given cgroup (4mb is needed for the runtime)
func (*Systemdv1Manager) GetSandboxCgroupPath(sbParent, sbID string) (string, string, error) {
	return getSandboxCgroupPathForSystemd(sbParent, sbID, cgroupMemorySubsystemMountPathV1, "memory.limit_in_bytes")
}

// MoveConmonToCgroup takes the container ID, cgroup parent, conmon's cgroup (from the config) and conmon's PID
// It attempts to move conmon to the correct cgroup.
func (*Systemdv1Manager) MoveConmonToCgroup(cid, cgroupParent, conmonCgroup string, pid int) string {
	return moveConmonToSystemdCgroup(cid, cgroupParent, conmonCgroup, pid)
}

type Systemdv2Manager struct{}

// Name returns the name of the cgroup manager (systemd)
func (*Systemdv2Manager) Name() string {
	return SystemdCgroupManager
}

// IsSystemd returns that it is a systemd cgroup manager
func (*Systemdv2Manager) IsSystemd() bool {
	return true
}

// GetContainerCgroupPath takes arguments sandbox parent cgroup and container ID and returns
// the cgroup path for that containerID. If parentCgroup is empty, it
// uses the default parent system.slice
func (*Systemdv2Manager) GetContainerCgroupPath(sbParent, containerID string) string {
	return getContainerCgroupPath(sbParent, containerID)
}

// systemd cgroups (v1 and v2) have no difference in creating the container cgroup path
func getContainerCgroupPath(sbParent, containerID string) string {
	parent := defaultSystemdParent
	if sbParent != "" {
		parent = sbParent
	}
	return parent + ":" + scopePrefix + ":" + containerID
}

// GetSandboxCgroupPath takes the sandbox parent, and sandbox ID. It
// returns the cgroup parent, cgroup path, and error.
// it also checks there is enough memory in the given cgroup (4mb is needed for the runtime)
func (*Systemdv2Manager) GetSandboxCgroupPath(sbParent, sbID string) (string, string, error) {
	return getSandboxCgroupPathForSystemd(sbParent, sbID, cgroupMemorySubsystemMountPathV2, "memory.max")
}

func getSandboxCgroupPathForSystemd(sbParent, sbID, memorySubsystemPath, memoryPath string) (string, string, error) {
	if sbParent == "" {
		return "", "", nil
	}

	if len(sbParent) <= 6 || !strings.HasSuffix(path.Base(sbParent), ".slice") {
		return "", "", fmt.Errorf("cri-o configured with systemd cgroup manager, but did not receive slice as parent: %s", sbParent)
	}

	cgroupParent := convertCgroupFsNameToSystemd(sbParent)

	if err := verifyCgroupHasEnoughMemory(cgroupParent, memorySubsystemPath, "memory.limit_in_bytes"); err != nil {
		return "", "", err
	}

	cgPath := cgroupParent + ":" + scopePrefix + ":" + sbID

	return cgroupParent, cgPath, nil
}

func verifyCgroupHasEnoughMemory(cgroupParent, memorySubsystemPath, memoryMaxFilename string) error {
	// check memory limit is greater than the minimum memory limit of 4Mb
	// expand the cgroup slice path
	slicePath, err := systemd.ExpandSlice(cgroupParent)
	if err != nil {
		return errors.Wrapf(err, "error expanding systemd slice path for %q", cgroupParent)
	}

	// read in the memory limit from the memory.limit_in_bytes file
	fileData, err := ioutil.ReadFile(filepath.Join(memorySubsystemPath, slicePath, memoryMaxFilename))
	if err != nil {
		if os.IsNotExist(err) {
			logrus.Warnf("Failed to find %s for slice: %q", memoryMaxFilename, cgroupParent)
			return nil
		} else {
			return errors.Wrapf(err, "error reading %s file for slice %q", memoryMaxFilename, cgroupParent)
		}
	}
	// strip off the newline character and convert it to an int
	strMemory := strings.TrimRight(string(fileData), "\n")
	if strMemory != "" && strMemory != "max" {
		memoryLimit, err := strconv.ParseInt(strMemory, 10, 64)
		if err != nil {
			return errors.Wrapf(err, "error converting cgroup memory value from string to int %q", strMemory)
		}
		// Compare with the minimum allowed memory limit
		if err := VerifyMemoryIsEnough(memoryLimit); err != nil {
			return fmt.Errorf("pod %s", err.Error())
		}
	}
	return nil
}

func VerifyMemoryIsEnough(memoryLimit int64) error {
	if memoryLimit != 0 && memoryLimit < minMemoryLimit {
		return fmt.Errorf("set memory limit %v too low; should be at least %v", memoryLimit, minMemoryLimit)
	}
	return nil
}

// convertCgroupFsNameToSystemd converts an expanded cgroupfs name to its systemd name.
// For example, it will convert test.slice/test-a.slice/test-a-b.slice to become test-a-b.slice
// NOTE: this is public right now to allow its usage in dockermanager and dockershim, ideally both those
// code areas could use something from libcontainer if we get this style function upstream.
func convertCgroupFsNameToSystemd(cgroupfsName string) string {
	// TODO: see if libcontainer systemd implementation could use something similar, and if so, move
	// this function up to that library.  At that time, it would most likely do validation specific to systemd
	// above and beyond the simple assumption here that the base of the path encodes the hierarchy
	// per systemd convention.
	return path.Base(cgroupfsName)
}

func (*Systemdv2Manager) MoveConmonToCgroup(cid, cgroupParent, conmonCgroup string, pid int) string {
	return moveConmonToSystemdCgroup(cid, cgroupParent, conmonCgroup, pid)
}

func moveConmonToSystemdCgroup(cid, cgroupParent, conmonCgroup string, pid int) string {
	if strings.HasSuffix(conmonCgroup, ".slice") {
		cgroupParent = conmonCgroup
	}
	conmonUnitName := createConmonUnitName(cid)
	logrus.Debugf("Running conmon under slice %s and unitName %s", cgroupParent, conmonUnitName)
	if err := utils.RunUnderSystemdScope(pid, cgroupParent, conmonUnitName); err != nil {
		logrus.Warnf("Failed to add conmon to systemd sandbox cgroup: %v", err)
	}
	return ""
}

func createConmonUnitName(name string) string {
	return createUnitName("crio-conmon", name)
}

func createUnitName(prefix, name string) string {
	return fmt.Sprintf("%s-%s.scope", prefix, name)
}
