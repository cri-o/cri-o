// +build linux

package node

import (
	"github.com/sirupsen/logrus"
)

// ValidateConfig initializes and validates all of the singleton variables
// that store the node's configuration.
// Currently, we check hugetlb, cgroup v1 or v2, pid and memory swap support for cgroups.
// We check the error at server configuration validation, and if we error, shutdown
// cri-o early, instead of when we're already trying to run containers.
func ValidateConfig() error {
	toInit := []struct {
		name string
		init func() bool
		err  *error
	}{
		{
			name: "hugetlb cgroup",
			init: CgroupHasHugetlb,
			err:  &cgroupControllerErr,
		},
		{
			name: "pid cgroup",
			init: CgroupHasPid,
			err:  &cgroupControllerErr,
		},
		{
			name: "memoryswap cgroup",
			init: CgroupHasMemorySwap,
			err:  &cgroupHasMemorySwapErr,
		},
	}
	for _, i := range toInit {
		i.init()
		if *i.err != nil {
			logrus.Errorf("node configuration validation for %s failed: %v", i.name, *i.err)
		}
	}
	return nil
}
