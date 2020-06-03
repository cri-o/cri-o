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
		name      string
		init      func() bool
		err       *error
		activated *bool
		fatal     bool
	}{
		{
			name:      "hugetlb cgroup",
			init:      CgroupHasHugetlb,
			err:       &cgroupControllerErr,
			activated: &cgroupHasHugetlb,
			fatal:     true,
		},
		{
			name:      "pid cgroup",
			init:      CgroupHasPid,
			err:       &cgroupControllerErr,
			activated: &cgroupHasPid,
			fatal:     true,
		},
		{
			name:      "memoryswap cgroup",
			init:      CgroupHasMemorySwap,
			err:       &cgroupHasMemorySwapErr,
			activated: &cgroupHasMemorySwap,
			fatal:     false,
		},
	}
	for _, i := range toInit {
		i.init()
		if *i.err != nil {
			err := errors.Errorf("node configuration validation for %s failed: %v", i.name, *i.err)
			if i.fatal {
				return err
			}
			logrus.Error(err.Error())
		}
		logrus.Infof("node configuration value for %s is %v", i.name, *i.activated)
	}
	return nil
}
