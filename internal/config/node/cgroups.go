// +build linux

package node

import (
	"io/ioutil"
	"os"
	"strings"
	"sync"

	"github.com/opencontainers/runc/libcontainer/cgroups"
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
)

var CgroupIsV2 = cgroups.IsCgroup2UnifiedMode

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

type controllerInfo struct {
	name    string
	enabled *bool
}

func checkRelevantControllers() {
	cgroupControllerOnce.Do(func() {
		relevantControllers := []controllerInfo{
			{
				name:    "pids",
				enabled: &cgroupHasPid,
			},
			{
				name:    "hugetlb",
				enabled: &cgroupHasHugetlb,
			},
		}

		if CgroupIsV2() {
			checkRelevantControllersV2(relevantControllers)
			return
		}
		checkRelevantControllersV1(relevantControllers)
	})
}

func checkRelevantControllersV1(relevantControllers []controllerInfo) {
	controllerStats, cgroupControllerErr := ioutil.ReadDir("/sys/fs/cgroup")
	if cgroupControllerErr != nil {
		return
	}
	for _, toCheck := range relevantControllers {
		for _, st := range controllerStats {
			if st.IsDir() && toCheck.name == st.Name() {
				*toCheck.enabled = true
				break
			}
		}
	}
}

func checkRelevantControllersV2(relevantControllers []controllerInfo) {
	controllerNames, cgroupControllerErr := ioutil.ReadFile("/sys/fs/cgroup/cgroup.controllers")
	if cgroupControllerErr != nil {
		return
	}
	for _, toCheck := range relevantControllers {
		for _, name := range strings.Fields(string(controllerNames)) {
			if toCheck.name == name {
				*toCheck.enabled = true
				break
			}
		}
	}
}
