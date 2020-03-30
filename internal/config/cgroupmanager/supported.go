// +build linux

package cgroupmanager

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
	_cgroupHasHugetlbOnce sync.Once
	_cgroupHasHugetlb     bool
	_cgroupHasHugetlbErr  error

	_cgroupHasPidOnce sync.Once
	_cgroupHasPid     bool
	_cgroupHasPidErr  error

	_cgroupHasMemorySwapOnce sync.Once
	_cgroupHasMemorySwap     bool
	_cgroupHasMemorySwapErr  error

	_cgroupIsV2Once sync.Once
	_cgroupIsV2     bool
	_cgroupIsV2Err  error
)

// initializeCgroups initializes all of the singleton variables
// that store the node's cgroup configuration. Currently, we check
// hugetlb, cgroup v1 or v2, pid and memory swap support
// We check the error at server initialization, and if we error, shutdown
// cri-o early, instead of when we're already trying to run containers.
func initializeCgroups() error {
	toInit := []struct {
		initializer func() bool
		err         *error
	}{
		{
			initializer: CgroupHasHugetlb,
			err:         &_cgroupHasHugetlbErr,
		},
		{
			initializer: CgroupHasPid,
			err:         &_cgroupHasPidErr,
		},
		{
			initializer: CgroupIsV2,
			err:         &_cgroupIsV2Err,
		},
		{
			initializer: CgroupHasMemorySwap,
			err:         &_cgroupHasMemorySwapErr,
		},
	}
	for _, i := range toInit {
		i.initializer()
		if *i.err != nil {
			return errors.Wrapf(*i.err, "failed to initialize cgroup information")
		}
	}
	return nil
}

// CgroupHasHugetlb returns whether the hugetlb controller is present
func CgroupHasHugetlb() bool {
	_cgroupHasHugetlbOnce.Do(func() {
		if CgroupIsV2() {
			if _, err := ioutil.ReadDir("/sys/fs/cgroup/hugetlb"); err != nil {
				_cgroupHasHugetlbErr = errors.Wrap(err, "readdir /sys/fs/cgroup/hugetlb")
				return
			}
			_cgroupHasHugetlb = true
			return
		}

		controllers, err := ioutil.ReadFile("/sys/fs/cgroup/cgroup.controllers")
		if err != nil {
			_cgroupHasHugetlbErr = errors.Wrap(err, "read /sys/fs/cgroup/cgroup.controllers")
			return
		}
		_cgroupHasHugetlb = strings.Contains(string(controllers), "hugetlb")
	})
	return _cgroupHasHugetlb
}

// CgroupHasPid returns whether the pid controller is present
func CgroupHasPid() bool {
	_cgroupHasPidOnce.Do(func() {
		_, err := libctrcgroups.FindCgroupMountpoint("", "pids")
		_cgroupHasPid = err == nil
		_cgroupHasPidErr = err
	})
	return _cgroupHasPid
}

// CgroupHasMemorySwap returns whether the memory swap controller is present
func CgroupHasMemorySwap() bool {
	_cgroupHasMemorySwapOnce.Do(func() {
		if CgroupIsV2() {
			_cgroupHasMemorySwap = true
			return
		}
		_, err := os.Stat("/sys/fs/cgroup/memory/memory.memsw.limit_in_bytes")
		_cgroupHasMemorySwap = err == nil
	})
	return _cgroupHasMemorySwap
}

// CgroupIsV2 returns whether we are using cgroup v2 or v1
func CgroupIsV2() bool {
	_cgroupIsV2Once.Do(func() {
		unified, err := libpodcgroups.IsCgroup2UnifiedMode()
		_cgroupIsV2 = unified
		_cgroupIsV2Err = err
	})
	return _cgroupIsV2
}
