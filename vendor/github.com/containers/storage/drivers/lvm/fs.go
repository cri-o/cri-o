package lvm

import (
	"os/exec"

	"github.com/sirupsen/logrus"
)

// FSMountOptions returns the list of hard-coded mount options which should be
// automatically added when mounting a filesystem of the specified type on the
// specified block device.
func FSMountOptions(fstype, device string) string {
	switch fstype {
	case "xfs":
		return "nouuid"
	}
	return ""
}

// FSNeededCommands checks that we have access to commands that are needed for
// a snapshotting a particular filesystem, and returns a list of what's needed
// but not found.
func FSNeededCommands(fstype string) []string {
	var needed, missing []string
	switch fstype {
	case "xfs":
		needed = []string{"xfs_admin"}
	case "ext2", "ext3", "ext4", "ext4dev":
		needed = []string{"tune2fs"}
	}
	for _, need := range needed {
		logrus.Debugf("checking for %q, needed for handling %q filesystems", need, fstype)
		if _, err := exec.LookPath(need); err != nil {
			logrus.Debugf("%q not found: %v", need, err)
			missing = append(missing, need)
		}
	}
	return missing
}

// FSPostSnapshotCmd returns a command line (suitable for use as an "arg" list
// when calling os/exec.Command) which should be run after a snapshot is
// created for a device containing the specified filesystem type.  It also
// expects the path of the block device.
func FSPostSnapshotCmd(fstype, device string) []string {
	switch fstype {
	case "xfs":
		return []string{"xfs_admin", "-U", "generate", device}
	case "ext2", "ext3", "ext4", "ext4dev":
		return []string{"tune2fs", "-U", "time", device}
	}
	return nil
}
