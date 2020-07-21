package utils

import (
	"os"
	"path/filepath"
	"syscall"
)

// GetDiskUsageStats accepts a path to a directory or file
// and returns the number of bytes and inodes used by the path
func GetDiskUsageStats(path string) (dirSize, inodeCount uint64, _ error) {
	if err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		// Walk does not follow symbolic links
		if err != nil {
			return err
		}

		dirSize += uint64(info.Size())
		inodeCount++

		return nil
	}); err != nil {
		return 0, 0, err
	}

	return dirSize, inodeCount, nil
}

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
