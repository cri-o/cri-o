// +build linux

package oci

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/containers/podman/v3/pkg/cgroups"
	"github.com/cri-o/cri-o/internal/config/node"
	"github.com/cri-o/cri-o/server/cri/types"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

func (r *runtimeOCI) createContainerPlatform(c *Container, cgroupParent string, pid int) error {
	if c.Spoofed() {
		return nil
	}
	g := &generate.Generator{
		Config: &rspec.Spec{
			Linux: &rspec.Linux{
				Resources: &rspec.LinuxResources{},
			},
		},
	}
	// Mutate our newly created spec to find the customizations that are needed for conmon
	if err := r.config.Workloads.MutateSpecGivenAnnotations(types.InfraContainerName, g, c.Annotations()); err != nil {
		return err
	}

	// Move conmon to specified cgroup
	conmonCgroupfsPath, err := r.config.CgroupManager().MoveConmonToCgroup(c.ID(), cgroupParent, r.config.ConmonCgroup, pid, g.Config.Linux.Resources)
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

	if ctr.Spoofed() {
		return stats, nil
	}

	// technically, the CRI does not mandate a CgroupParent is given to a pod
	// this situation should never happen in production, but some test suites
	// (such as critest) assume we can call stats on a cgroupless container
	if cgroup == "" {
		return stats, nil
	}
	// gets the real path of the cgroup on disk
	cgroupPath, err := r.config.CgroupManager().ContainerCgroupAbsolutePath(cgroup, ctr.ID())
	if err != nil {
		return nil, err
	}
	// checks cgroup just for the container, not the entire pod
	cg, err := cgroups.Load(cgroupPath)
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

	// Try our best to get the net namespace path.
	// If pid() errors, the container has stopped, and the /proc entry
	// won't exist anyway.
	pid, _ := ctr.pid() // nolint:errcheck
	if pid > 0 {
		netNsPath := fmt.Sprintf("/proc/%d/ns/net", pid)
		stats.NetInput, stats.NetOutput = getContainerNetIO(netNsPath)
	}

	totalInactiveFile, err := getTotalInactiveFile(cgroupPath)
	if err != nil { // nolint: gocritic
		logrus.Warnf("Error in memory working set stats retrieval: %v", err)
	} else if stats.MemUsage > totalInactiveFile {
		stats.WorkingSetBytes = stats.MemUsage - totalInactiveFile
	} else {
		logrus.Warnf(
			"unable to account working set stats: total_inactive_file (%d) > memory usage (%d)",
			totalInactiveFile, stats.MemUsage,
		)
	}

	return stats, nil
}

// getTotalInactiveFile returns the value if inactive_file as integer
// from cgroup's memory.stat. Returns an error if the file does not exists,
// not parsable, or the value is not found.
func getTotalInactiveFile(path string) (uint64, error) {
	var filename, varPrefix string
	if node.CgroupIsV2() {
		filename = filepath.Join("/sys/fs/cgroup", path, "memory.stat")
		varPrefix = "inactive_file "
	} else {
		filename = filepath.Join("/sys/fs/cgroup/memory", path, "memory.stat")
		varPrefix = "total_inactive_file "
	}
	f, err := os.Open(filename)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), varPrefix) {
			val, err := strconv.Atoi(
				strings.TrimPrefix(scanner.Text(), varPrefix),
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

	return 0, errors.Errorf("%q not found in %v", varPrefix, filename)
}
