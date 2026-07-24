//go:build linux

package utils

import (
	"os"
	"path/filepath"
	"syscall"
)

// GetDiskUsageStats accepts a path to a directory or file
// and returns the actual allocated disk blocks and inodes used by the path.
func GetDiskUsageStats(path string) (dirSize, inodeCount uint64, err error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return 0, 0, err
	}

	inodeCount++
	dirSize += getAllocatedSize(fi)

	if !fi.IsDir() {
		return dirSize, inodeCount, nil
	}

	s, i, err := calculateDir(path)

	return dirSize + s, inodeCount + i, err
}

func calculateDir(dirPath string) (dirSize, inodeCount uint64, err error) {
	f, err := os.Open(dirPath)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	entries, err := f.ReadDir(-1)
	if err != nil {
		return 0, 0, err
	}

	for _, entry := range entries {
		inodeCount++

		info, err := entry.Info()
		if err != nil {
			return 0, 0, err
		}

		dirSize += getAllocatedSize(info)

		if entry.IsDir() {
			subSize, subInodes, err := calculateDir(filepath.Join(dirPath, entry.Name()))
			if err != nil {
				return 0, 0, err
			}

			dirSize += subSize
			inodeCount += subInodes
		}
	}

	return dirSize, inodeCount, nil
}

func getAllocatedSize(fi os.FileInfo) uint64 {
	if sys := fi.Sys(); sys != nil {
		if stat, ok := sys.(*syscall.Stat_t); ok {
			return uint64(stat.Blocks) * 512
		}
	}

	return uint64(fi.Size())
}
