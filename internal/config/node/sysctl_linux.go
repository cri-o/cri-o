package node

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

var checkFsMayDetachMountsErr error

// checkFsMayDetachMounts is called once from ValidateConfig(),
// and its return value is ignored. It makes cri-o fail in case
// checkFsMayDetachMountsErr is not nil.
func checkFsMayDetachMounts() bool {
	// this sysctl is specific to RHEL7 kernel
	const file = "/proc/sys/fs/may_detach_mounts"

	data, err := os.ReadFile(file)
	if err != nil {
		if !os.IsNotExist(err) {
			logrus.WithError(err).Debug("checkFsMayDetachMounts")
		}

		return true
	}

	str := strings.TrimSpace(string(data))

	val, err := strconv.ParseInt(str, 10, 64)
	if err != nil { // should never happen
		logrus.WithError(err).Warnf("CheckFsMayDetachMounts: file %s, value %q, can't convert to int", file, str)

		return true
	}

	if val != 1 {
		checkFsMayDetachMountsErr = fmt.Errorf("fs.may_detach_mounts sysctl: expected 1, got %d; this may result in \"device or resource busy\" errors while stopping or removing containers", val)

		return false
	}

	return true
}
