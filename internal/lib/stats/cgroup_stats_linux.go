package stats

import "github.com/opencontainers/cgroups"

type CgroupStats struct {
	cgroups.Stats

	ProcessStats ProcessStats
	SystemNano   int64

	// IsCgroupV2 records the cgroup version that produced these stats. It is the
	// container's version, which for VM containers can differ from the host, so
	// consumers must use it rather than node.CgroupIsV2() to pick version
	// specific stat keys.
	IsCgroupV2 bool
}

type ProcessStats struct {
	Pids            []int
	FileDescriptors uint64
	Sockets         uint64
	Threads         uint64
	ThreadsMax      uint64
	UlimitsSoft     uint64
}
