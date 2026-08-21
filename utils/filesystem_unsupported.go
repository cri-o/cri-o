//go:build !linux

package utils

import (
	"os"
	"path/filepath"
)

// GetDiskUsageStats accepts a path to a directory or file
// and returns the number of bytes and inodes used by the path.
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
