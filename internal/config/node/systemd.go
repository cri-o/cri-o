// +build linux

package node

import (
	"os/exec"
	"regexp"
	"strconv"
	"sync"

	"github.com/pkg/errors"
)

var (
	systemdHasCollectModeOnce sync.Once
	systemdHasCollectMode     bool
	systemdHasCollectModeErr  error
)

func SystemdHasCollectMode() bool {
	systemdHasCollectModeOnce.Do(func() {
		stdout, err := exec.Command("systemctl", "--version").Output()
		if err != nil {
			systemdHasCollectModeErr = err
			return
		}
		matches := regexp.MustCompile(`^systemd (?P<Version>\d+) .*`).FindStringSubmatch(string(stdout))

		if len(matches) != 2 {
			systemdHasCollectModeErr = errors.Errorf("systemd version command returned incompatible formatted information: %v", string(stdout))
			return
		}

		systemdVersion, err := strconv.Atoi(matches[1])
		if err != nil {
			systemdHasCollectModeErr = err
			return
		}
		if systemdVersion >= 236 {
			systemdHasCollectMode = true
		}
	})
	return systemdHasCollectMode
}
