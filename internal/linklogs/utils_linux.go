package linklogs

import (
	"golang.org/x/sys/unix"
)

func mountLogPath(podLogsPath, emptyDirLoggingVolumePath string) error {
	// CRI-O needs writable host access for symlinks and SELinux labeling.
	// Read-only access is enforced on the container's OCI mounts instead.
	return unix.Mount(
		podLogsPath,
		emptyDirLoggingVolumePath,
		"bind",
		unix.MS_BIND,
		"",
	)
}

func unmountLogPath(emptyDirLoggingVolumePath string) error {
	return unix.Unmount(emptyDirLoggingVolumePath, unix.MNT_DETACH)
}
