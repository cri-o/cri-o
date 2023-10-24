package cgmgr

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/containers/common/pkg/cgroups"
	"github.com/cri-o/cri-o/internal/config/node"
	libctrcgroups "github.com/opencontainers/runc/libcontainer/cgroups"
	"github.com/sirupsen/logrus"
)

// CgroupStats is a structure used to augment libctr.Stats object, because
// it does not have all of the information required for all of the stats we're
// interested in.
type CgroupStats struct {
	MostStats     *libctrcgroups.Stats
	OtherMemStats *CgroupMemoryStats
	SystemNano    int64
}

// CgroupMemoryStats is an object to hold the memory stats from the memory.stat file
// of the given cgroup.
type CgroupMemoryStats struct {
	Rss            uint64
	PgFault        uint64
	PgMajFault     uint64
	WorkingSet     uint64
	AvailableBytes uint64
}

func cgroupStatsFromPath(cgroupPath string) (*CgroupStats, error) {
	cg, err := cgroups.Load(cgroupPath)
	if err != nil {
		return nil, fmt.Errorf("unable to load cgroup at %s: %w", cgroupPath, err)
	}

	cgroupStats, err := cg.Stat()
	if err != nil {
		return nil, fmt.Errorf("unable to get cgroup stats: %w", err)
	}

	memUsage := cgroupStats.MemoryStats.Usage.Usage

	memStats, err := memoryStatsGivenPath(cgroupPath, memUsage)
	if err != nil {
		return nil, fmt.Errorf("unable to update with memory.stat info: %w", err)
	}

	memLimit := MemLimitGivenSystem(cgroupStats.MemoryStats.Usage.Limit)
	memStats.AvailableBytes = memLimit - memUsage

	return &CgroupStats{
		MostStats:     cgroupStats,
		OtherMemStats: memStats,
		SystemNano:    time.Now().UnixNano(),
	}, nil
}

// memoryStatsGivenPath updates the CgroupMemoryStats object with info
// from cgroup's memory.stat. Returns an error if the file does not exists,
// or not parsable.
func memoryStatsGivenPath(path string, usage uint64) (*CgroupMemoryStats, error) {
	const memoryStatFile = "memory.stat"
	var memoryStatPath, inactiveFileSearchString string
	if !node.CgroupIsV2() {
		memoryStatPath = filepath.Join(cgroupMemoryPathV1, path, memoryStatFile)
		inactiveFileSearchString = "total_inactive_file "
	} else {
		memoryStatPath = filepath.Join(cgroupMemoryPathV2, path, memoryStatFile)
		inactiveFileSearchString = "inactive_file "
	}
	return MemoryStatsFromFile(memoryStatPath, inactiveFileSearchString, usage)
}

func MemoryStatsFromFile(memoryStatPath, inactiveFileSearchString string, usage uint64) (*CgroupMemoryStats, error) {
	var totalInactive uint64
	f, err := os.Open(memoryStatPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	memStat := &CgroupMemoryStats{}

	toUpdate := []struct {
		prefix string
		field  *uint64
	}{
		{inactiveFileSearchString, &totalInactive},
		{"rss ", &memStat.Rss},
		{"pgfault ", &memStat.PgFault},
		{"pgmajfault ", &memStat.PgMajFault},
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		for _, field := range toUpdate {
			if !strings.HasPrefix(scanner.Text(), field.prefix) {
				continue
			}
			val, err := strconv.ParseUint(
				strings.TrimPrefix(scanner.Text(), field.prefix), 10, 64,
			)
			if err != nil {
				return nil, fmt.Errorf("unable to parse %s: %w", field.prefix, err)
			}
			*field.field = val
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if usage > totalInactive {
		memStat.WorkingSet = usage - totalInactive
	} else {
		logrus.Warnf(
			"Unable to account working set stats: total_inactive_file (%d) > memory usage (%d)",
			totalInactive, usage,
		)
	}

	return memStat, nil
}

// MemLimitGivenSystem limit returns the memory limit for a given cgroup
// If the configured memory limit is larger than the total memory on the sys, the
// physical system memory size is returned
func MemLimitGivenSystem(cgroupLimit uint64) uint64 {
	si := &syscall.Sysinfo_t{}
	err := syscall.Sysinfo(si)
	if err != nil {
		return cgroupLimit
	}

	// conversion to uint64 needed to build on 32-bit
	// but lint complains about unnecessary conversion
	// see: pr#2409
	physicalMemory := uint64(si.Totalram) //nolint:unconvert

	// If the cgroup limit exceeds the available physical memory, use the physical memory as the limit
	if cgroupLimit > physicalMemory {
		return physicalMemory
	}
	return cgroupLimit
}
