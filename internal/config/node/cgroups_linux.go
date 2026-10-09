//go:build linux

package node

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/moby/sys/mountinfo"
	libctrcgroups "github.com/opencontainers/cgroups"
	"go.podman.io/common/pkg/cgroups"
)

const cgroupRoot = "/sys/fs/cgroup"

var (
	cgroupHasMemorySwapOnce sync.Once
	cgroupHasMemorySwap     bool
	cgroupHasMemorySwapErr  error

	cgroupControllerOnce sync.Once
	cgroupControllerErr  error
	cgroupHasHugetlb     bool
	cgroupHasPid         bool

	cgroupIsV2Err error
)

func CgroupIsV2() bool {
	var cgroupIsV2 bool

	cgroupIsV2, cgroupIsV2Err = cgroups.IsCgroup2UnifiedMode()

	return cgroupIsV2
}

// CgroupHasNsdelegate returns whether the cgroup v2 hierarchy is mounted with
// the nsdelegate option. It reads the mount options on each call because they
// can change while CRI-O runs.
func CgroupHasNsdelegate() (bool, error) {
	isV2, err := cgroups.IsCgroup2UnifiedMode()
	if err != nil || !isV2 {
		return false, err
	}

	return cgroupMountHasNsdelegate(mountinfo.GetMounts)
}

func cgroupMountHasNsdelegate(
	getMounts func(mountinfo.FilterFunc) ([]*mountinfo.Info, error),
) (bool, error) {
	mounts, err := getMounts(func(m *mountinfo.Info) (skip, stop bool) {
		return m.Mountpoint != cgroupRoot || m.FSType != "cgroup2", false
	})
	if err != nil {
		return false, err
	}

	for _, m := range mounts {
		if slices.Contains(strings.Split(m.VFSOptions, ","), "nsdelegate") {
			return true, nil
		}
	}

	return false, nil
}

// CgroupHasMemorySwap returns whether the memory swap controller is present.
func CgroupHasMemorySwap() bool {
	cgroupHasMemorySwapOnce.Do(func() {
		if CgroupIsV2() {
			cg, err := libctrcgroups.ParseCgroupFile("/proc/self/cgroup")
			if err != nil {
				cgroupHasMemorySwapErr = err
				cgroupHasMemorySwap = false

				return
			}

			memSwap := filepath.Join(cgroupRoot, cg[""], "memory.swap.current")
			if _, err := os.Stat(memSwap); err != nil {
				cgroupHasMemorySwap = false

				return
			}

			cgroupHasMemorySwap = true

			return
		}

		_, err := os.Stat("/sys/fs/cgroup/memory/memory.memsw.limit_in_bytes")
		if err != nil {
			cgroupHasMemorySwapErr = errors.New("node not configured with memory swap")
			cgroupHasMemorySwap = false

			return
		}

		cgroupHasMemorySwap = true
	})

	return cgroupHasMemorySwap
}

// CgroupHasHugetlb returns whether the hugetlb controller is present.
func CgroupHasHugetlb() bool {
	checkRelevantControllers()

	return cgroupHasHugetlb
}

// CgroupHasPid returns whether the pid controller is present.
func CgroupHasPid() bool {
	checkRelevantControllers()

	return cgroupHasPid
}

func checkRelevantControllers() {
	cgroupControllerOnce.Do(func() {
		relevantControllers := []struct {
			name    string
			enabled *bool
		}{
			{
				name:    "pids",
				enabled: &cgroupHasPid,
			},
			{
				name:    "hugetlb",
				enabled: &cgroupHasHugetlb,
			},
		}

		ctrls, err := libctrcgroups.GetAllSubsystems()
		if err != nil {
			cgroupControllerErr = err

			return
		}

		for _, toCheck := range relevantControllers {
			if slices.Contains(ctrls, toCheck.name) {
				*toCheck.enabled = true
			}
		}
	})
}
