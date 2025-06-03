package oci

import (
	"os"
	"syscall"

	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"

	"github.com/cri-o/cri-o/utils"
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

	// Record conmon's cgroup path in the container, so we can properly
	// clean it up when removing the container.
	c.conmonCgroupfsPath = conmonCgroupfsPath

	return nil
}

func sysProcAttrPlatform() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setpgid: true,
	}
}

// newPipe creates a unix socket pair for communication.
func newPipe() (parent, child *os.File, _ error) {
	fds, err := unix.Socketpair(unix.AF_LOCAL, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}

	return os.NewFile(uintptr(fds[1]), "parent"), os.NewFile(uintptr(fds[0]), "child"), nil
}
