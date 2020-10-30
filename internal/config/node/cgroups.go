// +build linux

package node

import (
	"os"
	"sync"

	libpodcgroups "github.com/containers/podman/v2/pkg/cgroups"
	libctrcgroups "github.com/opencontainers/runc/libcontainer/cgroups"
	"github.com/pkg/errors"
)

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
	cgroupIsV2, cgroupIsV2Err = libpodcgroups.IsCgroup2UnifiedMode()
	return cgroupIsV2
}

// CgroupHasMemorySwap returns whether the memory swap controller is present
func CgroupHasMemorySwap() bool {
	cgroupHasMemorySwapOnce.Do(func() {
		if CgroupIsV2() {
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

// CgroupHasHugetlb returns whether the hugetlb controller is present
func CgroupHasHugetlb() bool {
	checkRelevantControllers()
	return cgroupHasHugetlb
}

// CgroupHasPid returns whether the pid controller is present
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
			for _, ctrl := range ctrls {
				if ctrl == toCheck.name {
					*toCheck.enabled = true
					break
				}
			}
		}
	})
}
