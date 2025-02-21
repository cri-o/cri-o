package linklogs

import (
	"golang.org/x/sys/unix"
)

func mountLogPath(podLogsPath, emptyDirLoggingVolumePath string) error {
	return unix.Mount(podLogsPath, emptyDirLoggingVolumePath, "bind", unix.MS_BIND|unix.MS_RDONLY, "")
}

func unmountLogPath(emptyDirLoggingVolumePath string) error {
	return unix.Unmount(emptyDirLoggingVolumePath, unix.MNT_DETACH)
}
