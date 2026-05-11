package stats

import "github.com/opencontainers/cgroups"

type CgroupStats struct {
	cgroups.Stats

	ProcessStats ProcessStats
	SystemNano   int64
}

type ProcessStats struct {
	Pids            []int
	FileDescriptors uint64
	Sockets         uint64
	Threads         uint64
	ThreadsMax      uint64
	UlimitsSoft     uint64
}
