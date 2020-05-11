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

// SystemdManager is the parent type of SystemdV{1,2}Manager.
// it defines all of the common functionality between V1 and V2
type SystemdManager struct{}

// Name returns the name of the cgroup manager (systemd)
func (*SystemdManager) Name() string {
	return systemdCgroupManager
}

// IsSystemd returns that it is a systemd cgroup manager
func (*SystemdManager) IsSystemd() bool {
	return true
}

// GetContainerCgroupPath takes arguments sandbox parent cgroup and container ID and returns
// the cgroup path for that containerID. If parentCgroup is empty, it
// uses the default parent system.slice
func (*SystemdManager) GetContainerCgroupPath(sbParent, containerID string) string {
	parent := defaultSystemdParent
	if sbParent != "" {
		parent = sbParent
	}
	return parent + ":" + scopePrefix + ":" + containerID
}

// MoveConmonToCgroup takes the container ID, cgroup parent, conmon's cgroup (from the config) and conmon's PID
// It attempts to move conmon to the correct cgroup.
func (*SystemdManager) MoveConmonToCgroup(cid, cgroupParent, conmonCgroup string, pid int) (string, error) {
	if strings.HasSuffix(conmonCgroup, ".slice") {
		cgroupParent = conmonCgroup
	}
	conmonUnitName := fmt.Sprintf("crio-conmon-%s.scope", cid)
	logrus.Debugf("Running conmon under slice %s and unitName %s", cgroupParent, conmonUnitName)
	if err := utils.RunUnderSystemdScope(pid, cgroupParent, conmonUnitName); err != nil {
		return "", errors.Wrapf(err, "Failed to add conmon to systemd sandbox cgroup")
	}
	return "", nil
}

type SystemdV1Manager struct {
	SystemdManager
}

// GetSandboxCgroupPath takes the sandbox parent, and sandbox ID. It
// returns the cgroup parent, cgroup path, and error.
// it also checks there is enough memory in the given cgroup (4mb is needed for the runtime)
func (*SystemdV1Manager) GetSandboxCgroupPath(sbParent, sbID string) (cgParent, cgPath string, err error) {
	return getSandboxCgroupPathForSystemd(sbParent, sbID, cgroupMemorySubsystemMountPathV1, "memory.limit_in_bytes")
}

type SystemdV2Manager struct {
	SystemdManager
}

// GetSandboxCgroupPath takes the sandbox parent, and sandbox ID. It
// returns the cgroup parent, cgroup path, and error.
// it also checks there is enough memory in the given cgroup (4mb is needed for the runtime)
func (*SystemdV2Manager) GetSandboxCgroupPath(sbParent, sbID string) (cgParent, cgPath string, err error) {
	return getSandboxCgroupPathForSystemd(sbParent, sbID, cgroupMemorySubsystemMountPathV2, "memory.max")
}

func getSandboxCgroupPathForSystemd(sbParent, sbID, memorySubsystemPath, memoryMaxFile string) (cgParent, cgPath string, err error) {
	if sbParent == "" {
		return "", "", nil
	}

	if !strings.HasSuffix(filepath.Base(sbParent), ".slice") {
		return "", "", fmt.Errorf("cri-o configured with systemd cgroup manager, but did not receive slice as parent: %s", sbParent)
	}

	cgParent = convertCgroupFsNameToSystemd(sbParent)

	if err := verifyCgroupHasEnoughMemory(cgParent, memorySubsystemPath, memoryMaxFile); err != nil {
		return "", "", err
	}

	cgPath = cgParent + ":" + scopePrefix + ":" + sbID

	return cgParent, cgPath, nil
}

func verifyCgroupHasEnoughMemory(cgroupParent, memorySubsystemPath, memoryMaxFilename string) error {
	// check memory limit is greater than the minimum memory limit of 4Mb
	// expand the cgroup slice path
	slicePath, err := systemd.ExpandSlice(cgroupParent)
	if err != nil {
		return errors.Wrapf(err, "expanding systemd slice path for %q", cgroupParent)
	}

	// read in the memory limit from the memory.limit_in_bytes file
	fileData, err := ioutil.ReadFile(filepath.Join(memorySubsystemPath, slicePath, memoryMaxFilename))
	if err != nil {
		if os.IsNotExist(err) {
			logrus.Warnf("Failed to find %s for slice: %q", memoryMaxFilename, cgroupParent)
			return nil
		}
		return errors.Wrapf(err, "unable to read memory file for cgroup %s", cgroupParent)
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
			return errors.Errorf("pod %v", err)
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
func convertCgroupFsNameToSystemd(cgroupfsName string) string {
	// TODO: see if libcontainer systemd implementation could use something similar, and if so, move
	// this function up to that library.  At that time, it would most likely do validation specific to systemd
	// above and beyond the simple assumption here that the base of the path encodes the hierarchy
	// per systemd convention.
	return path.Base(cgroupfsName)
}
