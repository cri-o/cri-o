// +build linux

package node

import (
	"io/ioutil"
	"os"
	"strings"
	"sync"

	libpodcgroups "github.com/containers/libpod/pkg/cgroups"
	libctrcgroups "github.com/opencontainers/runc/libcontainer/cgroups"
	"github.com/pkg/errors"
)

var (
	cgroupHasHugetlbOnce sync.Once
	cgroupHasHugetlb     bool
	cgroupHasHugetlbErr  error

	cgroupHasPidOnce sync.Once
	cgroupHasPid     bool
	cgroupHasPidErr  error

	cgroupHasMemorySwapOnce sync.Once
	cgroupHasMemorySwap     bool
	cgroupHasMemorySwapErr  error

	cgroupIsV2Once sync.Once
	cgroupIsV2     bool
	cgroupIsV2Err  error
)

// CgroupHasHugetlb returns whether the hugetlb controller is present
func CgroupHasHugetlb() bool {
	cgroupHasHugetlbOnce.Do(func() {
		if CgroupIsV2() {
			controllers, err := ioutil.ReadFile("/sys/fs/cgroup/cgroup.controllers")
			if err != nil {
				cgroupHasHugetlbErr = errors.Wrap(err, "read /sys/fs/cgroup/cgroup.controllers")
				return
			}
			cgroupHasHugetlb = strings.Contains(string(controllers), "hugetlb")
			return
		}

		if _, err := ioutil.ReadDir("/sys/fs/cgroup/hugetlb"); err != nil {
			cgroupHasHugetlbErr = errors.Wrap(err, "readdir /sys/fs/cgroup/hugetlb")
			return
		}
		cgroupHasHugetlb = true
	})
	return cgroupHasHugetlb
}

// CgroupHasPid returns whether the pid controller is present
func CgroupHasPid() bool {
	cgroupHasPidOnce.Do(func() {
		_, err := libctrcgroups.FindCgroupMountpoint("", "pids")
		cgroupHasPid = err == nil
		cgroupHasPidErr = err
	})
	return cgroupHasPid
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

// CgroupIsV2 returns whether we are using cgroup v2 or v1
func CgroupIsV2() bool {
	cgroupIsV2Once.Do(func() {
		unified, err := libpodcgroups.IsCgroup2UnifiedMode()
		cgroupIsV2 = unified
		cgroupIsV2Err = err
	})
	return cgroupIsV2
}
