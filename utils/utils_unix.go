//go:build !linux

package utils

import (
	"golang.org/x/sys/unix"
)

// Syncfs ensures the file system at path is synced to disk. This seems to be a
// linux-specific syscall - for non-linux we have to sync all filesystems.
func Syncfs(path string) error {
	return unix.Sync()
}
