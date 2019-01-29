// +build linux

package oci

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/containerd/cgroups"
	"github.com/kubernetes-sigs/cri-o/utils"
	"github.com/opencontainers/runc/libcontainer"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

func createUnitName(prefix string, name string) string {
	return fmt.Sprintf("%s-%s.scope", prefix, name)
}

func (r *RuntimeOCI) createContainerPlatform(c *Container, cgroupParent string, pid int) error {
	// Move conmon to specified cgroup
	if r.cgroupManager == SystemdCgroupsManager {
		logrus.Debugf("Running conmon under slice %s and unitName %s", cgroupParent, createUnitName("crio-conmon", c.id))
		if err := utils.RunUnderSystemdScope(pid, cgroupParent, createUnitName("crio-conmon", c.id)); err != nil {
			logrus.Warnf("Failed to add conmon to systemd sandbox cgroup: %v", err)
		}
		return nil
	}

	control, err := cgroups.New(cgroups.V1, cgroups.StaticPath(filepath.Join(cgroupParent, "/crio-conmon-"+c.id)), &rspec.LinuxResources{})
	if err != nil {
		logrus.Warnf("Failed to add conmon to cgroupfs sandbox cgroup: %v", err)
		return nil
	}

	// Here we should defer a crio-connmon- cgroup hierarchy deletion, but it will
	// always fail as conmon's pid is still there.
	// Fortunately, kubelet takes care of deleting this for us, so the leak will
	// only happens in corner case where one does a manual deletion of the container
	// through e.g. runc. This should be handled by implementing a conmon monitoring
	// routine that does the cgroup cleanup once conmon is terminated.
	if err := control.Add(cgroups.Process{Pid: pid}); err != nil {
		logrus.Warnf("Failed to add conmon to cgroupfs sandbox cgroup: %v", err)
	}
	return nil
}

func sysProcAttrPlatform() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setpgid: true,
	}
}

// newPipe creates a unix socket pair for communication
func newPipe() (parent *os.File, child *os.File, err error) {
	fds, err := unix.Socketpair(unix.AF_LOCAL, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	return os.NewFile(uintptr(fds[1]), "parent"), os.NewFile(uintptr(fds[0]), "child"), nil
}

func loadFactory(root string) (libcontainer.Factory, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	cgroupManager := libcontainer.Cgroupfs
	return libcontainer.New(abs, cgroupManager, libcontainer.CriuPath(""))
}

// libcontainerStats gets the stats for the container with the given id from runc/libcontainer
func libcontainerStats(ctr *Container) (*libcontainer.Stats, error) {
	// TODO: make this not hardcoded
	// was: c.runtime.Path(ociContainer) but that returns /usr/bin/runc - how do we get /run/runc?
	// runroot is /var/run/runc
	// Hardcoding probably breaks Kata Containers compatibility
	factory, err := loadFactory("/run/runc")
	if err != nil {
		return nil, err
	}
	container, err := factory.Load(ctr.ID())
	if err != nil {
		return nil, err
	}
	return container.Stats()
}

func containerStats(ctr *Container) (*ContainerStats, error) {
	libcontainerStats, err := libcontainerStats(ctr)
	if err != nil {
		return nil, err
	}
	cgroupStats := libcontainerStats.CgroupStats
	stats := new(ContainerStats)
	stats.Container = ctr.ID()
	stats.CPUNano = cgroupStats.CpuStats.CpuUsage.TotalUsage
	stats.SystemNano = time.Now().UnixNano()
	stats.CPU = calculateCPUPercent(libcontainerStats)
	stats.MemUsage = cgroupStats.MemoryStats.Usage.Usage
	stats.MemLimit = getMemLimit(cgroupStats.MemoryStats.Usage.Limit)
	stats.MemPerc = float64(stats.MemUsage) / float64(stats.MemLimit)
	stats.PIDs = cgroupStats.PidsStats.Current
	stats.BlockInput, stats.BlockOutput = calculateBlockIO(libcontainerStats)
	stats.NetInput, stats.NetOutput = getContainerNetIO(libcontainerStats)

	return stats, nil
}
