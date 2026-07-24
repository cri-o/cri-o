//go:build linux

package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// GetRealPhysicalUsage returns actual disk usage accounting for shared extents (reflinks).
// This uses the FIEMAP ioctl to get physical extent mappings and tracks which blocks
// are shared between files to avoid double-counting deduplicated data.
//
// This is significantly more expensive than GetDiskUsageStats as it requires FIEMAP
// ioctl calls for every regular file. Use this when accurate reflink-aware reporting
// is critical (e.g., when containers-storage dedup is active).
//
// Returns bytes of unique physical blocks and inode count.
func GetRealPhysicalUsage(path string) (uniqueBytes, inodeCount uint64, err error) {
	seenExtents := make(map[uint64]uint64)

	err = filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			// Skip files that disappeared during traversal
			if os.IsNotExist(err) {
				return nil
			}

			return err
		}

		inodeCount++

		if !info.Mode().IsRegular() {
			return nil
		}

		extents, err := getFileExtents(p)
		if err != nil {
			if sys := info.Sys(); sys != nil {
				if stat, ok := sys.(*syscall.Stat_t); ok {
					uniqueBytes += uint64(stat.Blocks) * 512
				} else {
					uniqueBytes += uint64(info.Size())
				}
			} else {
				uniqueBytes += uint64(info.Size())
			}

			return nil
		}

		for _, ext := range extents {
			if ext.Physical == 0 {
				continue
			}

			if existingLen, seen := seenExtents[ext.Physical]; seen {
				if ext.Length > existingLen {
					uniqueBytes += ext.Length - existingLen
					seenExtents[ext.Physical] = ext.Length
				}
			} else {
				uniqueBytes += ext.Length
				seenExtents[ext.Physical] = ext.Length
			}
		}

		return nil
	})

	return uniqueBytes, inodeCount, err
}

// FiemapExtent represents a single extent from FIEMAP.
type FiemapExtent struct {
	Logical  uint64
	Physical uint64
	Length   uint64
	Flags    uint32
}

func getFileExtents(path string) ([]FiemapExtent, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}

	if stat.Size() == 0 {
		return nil, nil
	}

	const (
		fiemapMaxExtents = 512
		fiemapFlagSync   = 0x00000001
	)

	type fiemapExtent struct {
		feLogical  uint64
		fePhysical uint64
		feLength   uint64
		_          [2]uint64
		feFlags    uint32
		_          [3]uint32
	}

	type fiemap struct {
		fmStart         uint64
		fmLength        uint64
		fmFlags         uint32
		fmMappedExtents uint32
		fmExtentCount   uint32
		_               uint32
		fmExtents       [fiemapMaxExtents]fiemapExtent
	}

	var fm fiemap

	fm.fmStart = 0
	fm.fmLength = uint64(stat.Size())
	fm.fmFlags = fiemapFlagSync
	fm.fmExtentCount = fiemapMaxExtents

	const FS_IOC_FIEMAP = 0xC020660B

	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		f.Fd(),
		FS_IOC_FIEMAP,
		uintptr(unsafe.Pointer(&fm)),
	)

	if errno != 0 {
		return nil, fmt.Errorf("FIEMAP ioctl failed: %w", errno)
	}

	extents := make([]FiemapExtent, fm.fmMappedExtents)
	for i := range fm.fmMappedExtents {
		extents[i] = FiemapExtent{
			Logical:  fm.fmExtents[i].feLogical,
			Physical: fm.fmExtents[i].fePhysical,
			Length:   fm.fmExtents[i].feLength,
			Flags:    fm.fmExtents[i].feFlags,
		}
	}

	return extents, nil
}
