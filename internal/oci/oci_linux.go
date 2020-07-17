// +build linux

package oci

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/containers/libpod/v2/pkg/cgroups"
	"github.com/cri-o/cri-o/internal/config/node"
	"github.com/opencontainers/runc/libcontainer/cgroups/systemd"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

func (r *runtimeOCI) createContainerPlatform(c *Container, cgroupParent string, pid int) error {
	// Move conmon to specified cgroup
	conmonCgroupfsPath, err := r.config.CgroupManager().MoveConmonToCgroup(c.id, cgroupParent, r.config.ConmonCgroup, pid)
	if err != nil {
		return err
	}
	c.conmonCgroupfsPath = conmonCgroupfsPath
	return nil
}

func sysProcAttrPlatform() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setpgid: true,
	}
}

// newPipe creates a unix socket pair for communication
func newPipe() (parent, child *os.File, _ error) {
	fds, err := unix.Socketpair(unix.AF_LOCAL, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	return os.NewFile(uintptr(fds[1]), "parent"), os.NewFile(uintptr(fds[0]), "child"), nil
}

func (r *runtimeOCI) containerStats(ctr *Container, cgroup string) (*ContainerStats, error) {
	stats := &ContainerStats{}
	var err error
	stats.Container = ctr.ID()
	stats.SystemNano = time.Now().UnixNano()

	// technically, the CRI does not mandate a CgroupParent is given to a pod
	// this situation should never happen in production, but some test suites
	// (such as critest) assume we can call stats on a cgroupless container
	if cgroup == "" {
		return stats, nil
	}

	// this correction has to be made because the libpod cgroups package can't find a
	// systemd cgroup that isn't converted to a fully qualified cgroup path
	if r.config.CgroupManager().IsSystemd() {
		logrus.Debugf("Expanding systemd cgroup slice %v", cgroup)
		cgroup, err = systemd.ExpandSlice(cgroup)
		if err != nil {
			return nil, errors.Wrapf(err, "error expanding systemd slice to get container %s stats", ctr.ID())
		}
	}

	cg, err := cgroups.Load(cgroup)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to load cgroup at %s", cgroup)
	}

	cgroupStats, err := cg.Stat()
	if err != nil {
		return nil, errors.Wrap(err, "unable to obtain cgroup stats")
	}

	stats.CPUNano = cgroupStats.CPU.Usage.Total
	stats.CPU = calculateCPUPercent(cgroupStats)
	stats.MemUsage = cgroupStats.Memory.Usage.Usage
	stats.MemLimit = getMemLimit(cgroupStats.Memory.Usage.Limit)
	stats.MemPerc = float64(stats.MemUsage) / float64(stats.MemLimit)
	stats.PIDs = cgroupStats.Pids.Current
	stats.BlockInput, stats.BlockOutput = calculateBlockIO(cgroupStats)

	if ctr.state != nil {
		netNsPath := fmt.Sprintf("/proc/%d/ns/net", ctr.state.Pid)
		stats.NetInput, stats.NetOutput = getContainerNetIO(netNsPath)
	}

	totalInactiveFile, err := getTotalInactiveFile()
	if err != nil { // nolint: gocritic
		logrus.Warnf("error in memory working set stats retrieval: %v", err)
	} else if stats.MemUsage > totalInactiveFile {
		stats.WorkingSetBytes = stats.MemUsage - totalInactiveFile
	} else {
		logrus.Debugf(
			"unable to account working set stats: total_inactive_file (%d) > memory usage (%d)",
			totalInactiveFile, stats.MemUsage,
		)
	}

	return stats, nil
}

func metricsToCtrStats(c *Container, m *cgroups.Metrics) *ContainerStats {
	var (
		cpu         float64
		cpuNano     uint64
		memUsage    uint64
		memLimit    uint64
		memPerc     float64
		netInput    uint64
		netOutput   uint64
		blockInput  uint64
		blockOutput uint64
		pids        uint64
	)

	if m != nil {
		pids = m.Pids.Current

		cpuNano = m.CPU.Usage.Total
		cpu = genericCalculateCPUPercent(cpuNano, m.CPU.Usage.PerCPU)

		memUsage = m.Memory.Usage.Usage
		memLimit = getMemLimit(m.Memory.Usage.Limit)
		memPerc = float64(memUsage) / float64(memLimit)

		for _, entry := range m.Blkio.IoServiceBytesRecursive {
			switch strings.ToLower(entry.Op) {
			case "read":
				blockInput += entry.Value
			case "write":
				blockOutput += entry.Value
			}
		}
	}

	return &ContainerStats{
		Container:   c.ID(),
		CPU:         cpu,
		CPUNano:     cpuNano,
		SystemNano:  time.Now().UnixNano(),
		MemUsage:    memUsage,
		MemLimit:    memLimit,
		MemPerc:     memPerc,
		NetInput:    netInput,
		NetOutput:   netOutput,
		BlockInput:  blockInput,
		BlockOutput: blockOutput,
		PIDs:        pids,
	}
}

// getTotalInactiveFile returns the value if `total_inactive_file` as integer
// from `/sys/fs/cgroup/memory/memory.stat`. It returns an error if the file is
// not parsable.
func getTotalInactiveFile() (uint64, error) {
	// TODO: no cgroupv2 support right now
	if node.CgroupIsV2() {
		return 0, nil
	}

	const memoryStat = "/sys/fs/cgroup/memory/memory.stat"
	const totalInactiveFilePrefix = "total_inactive_file "
	f, err := os.Open(memoryStat)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), totalInactiveFilePrefix) {
			val, err := strconv.Atoi(
				strings.TrimPrefix(scanner.Text(), totalInactiveFilePrefix),
			)
			if err != nil {
				return 0, errors.Wrap(err, "unable to parse total inactive file value")
			}
			return uint64(val), nil
		}
	}

	if err := scanner.Err(); err != nil {
		return 0, err
	}

	return 0, errors.Errorf("%q not found in %v", totalInactiveFilePrefix, memoryStat)
}
