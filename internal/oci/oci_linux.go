package oci

import (
	"os"
	"syscall"
	"time"

	"github.com/cri-o/cri-o/utils"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

const InfraContainerName = "POD"

func (r *runtimeOCI) createContainerPlatform(c *Container, cgroupParent string, pid int) error {
	g := &generate.Generator{
		Config: &rspec.Spec{
			Linux: &rspec.Linux{
				Resources: &rspec.LinuxResources{},
			},
		},
	}

	// First, set the cpuset as the one for the infra container.
	// This should be overridden if specified in a workload.
	// It should not be applied unless the conmon cgroup is "pod".
	// Otherwise, the cpuset will be configured for whatever cgroup the conmons share
	// (which by default is system.slice).
	if r.config.InfraCtrCPUSet != "" && r.handler.MonitorCgroup == utils.PodCgroupName {
		logrus.Debugf("Set the conmon cpuset to %q", r.config.InfraCtrCPUSet)
		g.SetLinuxResourcesCPUCpus(r.config.InfraCtrCPUSet)
	}

	// Mutate our newly created spec to find the customizations that are needed for conmon
	if err := r.config.Workloads.MutateSpecGivenAnnotations(InfraContainerName, g, c.Annotations()); err != nil {
		return err
	}

	// Move conmon to specified cgroup
	conmonCgroupfsPath, err := r.config.CgroupManager().MoveConmonToCgroup(c.ID(), cgroupParent, r.handler.MonitorCgroup, pid, g.Config.Linux.Resources)
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

func (r *runtimeOCI) containerStats(ctr *Container, cgroup string) (*types.ContainerStats, error) {
	stats := &types.ContainerStats{
		Attributes: ctr.CRIAttributes(),
	}

	if ctr.Spoofed() {
		return stats, nil
	}

	// technically, the CRI does not mandate a CgroupParent is given to a pod
	// this situation should never happen in production, but some test suites
	// (such as critest) assume we can call stats on a cgroupless container
	if cgroup == "" {
		systemNano := time.Now().UnixNano()
		stats.Cpu = &types.CpuUsage{
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
	// update the stats object with information from the cgroup
	if err := r.config.CgroupManager().PopulateContainerCgroupStats(cgroup, ctr.ID(), stats); err != nil {
		return nil, err
	}
	return stats, nil
}
