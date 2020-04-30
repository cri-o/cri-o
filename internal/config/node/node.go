// +build linux

package node

import (
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ValidateConfig initializes and validates all of the singleton variables
// that store the node's configuration.
// Currently, we check hugetlb, cgroup v1 or v2, pid and memory swap support for cgroups.
// We check the error at server configuration validation, and if we error, shutdown
// cri-o early, instead of when we're already trying to run containers.
func ValidateConfig() error {
	toInit := []struct {
		name        string
		initializer func() bool
		err         *error
		fatal       bool
	}{
		{
			name:        "hugetlb cgroup",
			initializer: CgroupHasHugetlb,
			err:         &cgroupHasHugetlbErr,
			fatal:       false,
		},
		{
			name:        "pid cgroup",
			initializer: CgroupHasPid,
			err:         &cgroupHasPidErr,
			fatal:       false,
		},
		{
			name:        "cgroupv2",
			initializer: CgroupIsV2,
			err:         &cgroupIsV2Err,
			fatal:       false,
		},
		{
			name:        "memoryswap cgroup",
			initializer: CgroupHasMemorySwap,
			err:         &cgroupHasMemorySwapErr,
			fatal:       false,
		},
	}
	for _, i := range toInit {
		i.initializer()
		if *i.err != nil {
			err := errors.Wrapf(*i.err, "node configuration validation for %s failed", i.name)
			if i.fatal {
				return err
			}
			logrus.Error(err.Error())
		}
	}
	return nil
}
