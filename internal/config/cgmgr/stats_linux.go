package cgmgr

import (
	"context"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/opencontainers/cgroups"

	"github.com/cri-o/cri-o/internal/lib/stats"
	"github.com/cri-o/cri-o/internal/log"
)

func statsFromLibctrMgr(cgMgr cgroups.Manager) (*stats.CgroupStats, error) {
	cgStats, err := cgMgr.GetStats()
	if err != nil {
		return nil, err
	}

	pids, err := cgMgr.GetPids()
	if err != nil {
		return nil, err
	}

	return &stats.CgroupStats{
		Stats:        *cgStats,
		SystemNano:   time.Now().UnixNano(),
		ProcessStats: *cgroupProcessStats(cgStats, pids),
	}, nil
}

func cgroupProcessStats(cgroupStats *cgroups.Stats, pids []int) *stats.ProcessStats {
	var fdCount, socketCount, ulimitsSoft uint64

	// This is based on the cadvisor handler: https://github.com/google/cadvisor/blob/master/container/libcontainer/handler.go
	for _, pid := range pids {
		addFdsForProcess(pid, &fdCount, &socketCount)
		addUlimitsForProcess(pid, &ulimitsSoft)
	}

	return &stats.ProcessStats{
		Pids:            pids,
		FileDescriptors: fdCount,
		Sockets:         socketCount,
		Threads:         cgroupStats.PidsStats.Current,
		ThreadsMax:      cgroupStats.PidsStats.Limit,
		UlimitsSoft:     ulimitsSoft,
	}
}

func addFdsForProcess(pid int, fdCount, socketCount *uint64) {
	if fdCount == nil || socketCount == nil {
		panic("Programming error: fdCount or socketCount should not be nil")
	}

	dirPath := path.Join("/proc", strconv.Itoa(pid), "fd")

	fds, err := os.ReadDir(dirPath)
	if err != nil {
		log.Infof(context.Background(), "error while listing directory %q to measure fd count: %v", dirPath, err)

		return
	}

	*fdCount += uint64(len(fds))
	for _, fd := range fds {
		fdPath := path.Join(dirPath, fd.Name())

		linkName, err := os.Readlink(fdPath)
		if err != nil {
			log.Infof(context.Background(), "error while reading %q link: %v", fdPath, err)

			continue
		}

		if strings.HasPrefix(linkName, "socket") {
			*socketCount++
		}
	}
}

func addUlimitsForProcess(pid int, limits *uint64) {
	if limits == nil {
		panic("Programming error: limits should not be nil")
	}

	limitsPath := path.Join("/proc", strconv.Itoa(pid), "limits")

	limitsData, err := os.ReadFile(limitsPath)
	if err != nil {
		log.Infof(context.Background(), "error while reading %q to get thread limits: %v", limitsPath, err)

		return
	}

	for line := range strings.SplitSeq(string(limitsData), "\n") {
		if !strings.HasPrefix(line, "Max open files") {
			continue
		}

		const maxOpenFilesPrefix = "Max open files"

		remainingLine := strings.TrimSpace(line[len(maxOpenFilesPrefix):])

		fields := strings.Fields(remainingLine)
		if len(fields) >= 1 {
			if softLimit, err := strconv.ParseUint(fields[0], 10, 64); err == nil {
				*limits = softLimit
			}
		}

		return
	}
}
