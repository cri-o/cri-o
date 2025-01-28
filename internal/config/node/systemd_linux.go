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
)

func SystemdHasAllowedCPUs() bool {
	systemdHasAllowedCPUsOnce.Do(func() {
		systemdHasAllowedCPUs, systemdHasAllowedCPUsErr = systemdSupportsProperty("AllowedCPUs")
	})

	return systemdHasAllowedCPUs
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
