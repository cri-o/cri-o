package utils

import (
	"os"
	"syscall"
)

// IsDirectory tests whether the given path exists and is a directory. It
// follows symlinks.
func IsDirectory(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	if !info.Mode().IsDir() {
		// Return a PathError to be consistent with os.Stat().
		return &os.PathError{
			Op:   "stat",
			Path: path,
			Err:  syscall.ENOTDIR,
		}
	}

	return nil
}
