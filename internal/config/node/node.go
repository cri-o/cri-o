//go:build linux
// +build linux

package node

import (
	"fmt"

	"github.com/sirupsen/logrus"
)

// ValidateConfig initializes and validates all of the singleton variables
// that store the node's configuration.
// Currently, we check hugetlb, cgroup v1 or v2, pid and memory swap support for cgroups.
// We check the error at server configuration validation, and if we error, shutdown
// cri-o early, instead of when we're already trying to run containers.
func ValidateConfig() error {
	cgroupIsV2 := CgroupIsV2()
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
		{
			name:      "cgroup v2",
			init:      CgroupIsV2,
			err:       &cgroupIsV2Err,
			activated: &cgroupIsV2,
			fatal:     false,
		},
		{
			name:      "systemd CollectMode",
			init:      SystemdHasCollectMode,
			err:       &systemdHasCollectModeErr,
			activated: &systemdHasCollectMode,
			fatal:     false,
		},
		{
			name:      "systemd AllowedCPUs",
			init:      SystemdHasAllowedCPUs,
			err:       &systemdHasAllowedCPUsErr,
			activated: &systemdHasAllowedCPUs,
			fatal:     false,
		},
		{
			name:      "fs.may_detach_mounts sysctl",
			init:      checkFsMayDetachMounts,
			err:       &checkFsMayDetachMountsErr,
			activated: nil,
			fatal:     true,
		},
	}
	for _, i := range toInit {
		i.init()
		if *i.err != nil {
			err := fmt.Errorf("node configuration validation for %s failed: %w", i.name, *i.err)
			if i.fatal {
				return err
			}
			logrus.Warn(err)
		}
		if i.activated != nil {
			logrus.Infof("Node configuration value for %s is %v", i.name, *i.activated)
		}
	}
	return nil
}
