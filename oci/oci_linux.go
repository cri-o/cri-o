// +build linux

package oci

import (
	"os"
	"path/filepath"
	"syscall"

	"github.com/containerd/cgroups"
	"github.com/kubernetes-incubator/cri-o/utils"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

func (r *Runtime) createContainerPlatform(c *Container, cgroupParent string, pid int) error {
	// Move conmon to specified cgroup
	if r.cgroupManager == SystemdCgroupsManager {
		logrus.Infof("Running conmon under slice %s and unitName %s", cgroupParent, createUnitName("crio-conmon", c.id))
		return utils.RunUnderSystemdScope(pid, cgroupParent, createUnitName("crio-conmon", c.id))
	}

	control, err := cgroups.New(cgroups.V1, cgroups.StaticPath(filepath.Join(cgroupParent, "/crio-conmon-"+c.id)), &rspec.LinuxResources{})
	if err != nil {
		return err
	}

	// Here we should defer a crio-connmon- cgroup hierarchy deletion, but it will
	// always fail as conmon's pid is still there.
	// Fortunately, kubelet takes care of deleting this for us, so the leak will
	// only happens in corner case where one does a manual deletion of the container
	// through e.g. runc. This should be handled by implementing a conmon monitoring
	// routine that does the cgroup cleanup once conmon is terminated.
	return control.Add(cgroups.Process{Pid: pid})
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
