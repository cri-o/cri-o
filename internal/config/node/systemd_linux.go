//go:build linux

package node

import (
	"fmt"
	"sync"

	"github.com/cri-o/cri-o/utils/cmdrunner"
)

var (
	systemdHasAllowedCPUsOnce sync.Once
	systemdHasAllowedCPUs     bool
	systemdHasAllowedCPUsErr  error

	systemdHasCollectModeOnce sync.Once
	systemdHasCollectMode     bool
	systemdHasCollectModeErr  error
)

func SystemdHasAllowedCPUs() bool {
	systemdHasAllowedCPUsOnce.Do(func() {
		systemdHasAllowedCPUs, systemdHasAllowedCPUsErr = systemdSupportsProperty("AllowedCPUs")
	})

	return systemdHasAllowedCPUs
}

// SystemdHasCollectMode returns whether the running systemd supports the
// CollectMode unit property (added in systemd v236). Older systemd versions
// (e.g. unpatched RHEL 7) reject the property outright, which would fail the
// whole StartTransientUnit call used to place conmon in its scope.
func SystemdHasCollectMode() bool {
	systemdHasCollectModeOnce.Do(func() {
		systemdHasCollectMode, systemdHasCollectModeErr = systemdSupportsProperty("CollectMode")
	})

	return systemdHasCollectMode
}

// systemdSupportsProperty checks whether systemd supports a property
// It returns an error if it does not.
func systemdSupportsProperty(property string) (bool, error) {
	output, err := cmdrunner.Command("systemctl", "show", "-p", property, "systemd").Output()
	if err != nil {
		return false, fmt.Errorf("check systemd %s: %w", property, err)
	}

	if len(output) == 0 {
		return false, nil
	}

	return true, nil
}
