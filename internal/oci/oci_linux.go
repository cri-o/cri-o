package oci

import (
	"bufio"
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
	stats.Container = ctr.ID()
	stats.SystemNano = time.Now().UnixNano()

	if ctr.Spoofed() {
		return stats, nil
	}

	// technically, the CRI does not mandate a CgroupParent is given to a pod
	// this situation should never happen in production, but some test suites
	// (such as critest) assume we can call stats on a cgroupless container
	if cgroup == "" {
		systemNano := time.Now().UnixNano()
		stats.CPU = &types.CPUUsage{
			Timestamp: systemNano,
		}
		stats.Memory = &types.MemoryUsage{
			Timestamp: systemNano,
		}
		stats.WritableLayer = &types.FilesystemUsage{
			Timestamp: systemNano,
		}
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
	stats.MemUsage = cgroupStats.Memory.Usage.Usage
	memLimit := getMemLimit(cgroupStats.Memory.Usage.Limit)

	if err := updateWithMemoryStats(cgroupPath, stats); err != nil {
		return nil, errors.Wrap(err, "unable to update with memory.stat info")
	}
	stats.AvailableBytes = memLimit - stats.MemUsage

	return stats, nil
}

// updateWithMemoryStats updates the ContainerStats object with info
// from cgroup's memory.stat. Returns an error if the file does not exists,
// or not parsable.
func updateWithMemoryStats(path string, stats *ContainerStats) error {
	var filename, inactive string
	var totalInactive uint64
	if node.CgroupIsV2() {
		filename = filepath.Join("/sys/fs/cgroup", path, "memory.stat")
		inactive = "inactive_file "
	} else {
		filename = filepath.Join("/sys/fs/cgroup/memory", path, "memory.stat")
		inactive = "total_inactive_file "
	}

	toUpdate := []struct {
		prefix string
		field  *uint64
	}{
		{inactive, &totalInactive},
		{"rss ", &stats.RssBytes},
		{"pgfault ", &stats.PageFaults},
		{"pgmajfault ", &stats.MajorPageFaults},
	}

	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		for _, field := range toUpdate {
			if !strings.HasPrefix(scanner.Text(), field.prefix) {
				continue
			}
			val, err := strconv.Atoi(
				strings.TrimPrefix(scanner.Text(), field.prefix),
			)
			if err != nil {
				return errors.Wrapf(err, "unable to parse %s", field.prefix)
			}
			valUint := uint64(val)
			field.field = &valUint
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if stats.MemUsage > totalInactive {
		stats.WorkingSetBytes = stats.MemUsage - totalInactive
	} else {
		logrus.Warnf(
			"unable to account working set stats: total_inactive_file (%d) > memory usage (%d)",
			totalInactive, stats.MemUsage,
		)
	}

	return nil
}
