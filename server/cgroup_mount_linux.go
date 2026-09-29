package server

import (
	"errors"
	"fmt"
	"slices"

	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func cgroupMount(accessOption string) rspec.Mount {
	return rspec.Mount{
		Destination: cgroupSysFsPath,
		Type:        "cgroup",
		Source:      "cgroup",
		Options:     []string{"nosuid", "noexec", "nodev", "relatime", accessOption},
	}
}

func validateCgroupMountMode(config *types.ContainerConfig) error {
	securityContext := config.GetLinux().GetSecurityContext()

	switch mode := securityContext.GetCgroupMountMode(); mode {
	case types.CgroupMountMode_CGROUP_MOUNT_MODE_UNSPECIFIED:
		return nil
	case types.CgroupMountMode_CGROUP_MOUNT_MODE_READ_ONLY:
		if securityContext.GetPrivileged() {
			return errors.New("read-only cgroups are not supported for privileged containers")
		}

		return nil
	default:
		return fmt.Errorf("unsupported cgroup mount mode %q", mode)
	}
}

func applyCgroupMountMode(specgen *generate.Generator, config *types.ContainerConfig) {
	var accessOption string

	switch config.GetLinux().GetSecurityContext().GetCgroupMountMode() {
	case types.CgroupMountMode_CGROUP_MOUNT_MODE_READ_ONLY:
		accessOption = "ro"
	default:
		return
	}

	i := slices.IndexFunc(specgen.Config.Mounts, func(m rspec.Mount) bool {
		return m.Destination == cgroupSysFsPath && m.Type == "cgroup"
	})
	if i < 0 {
		return
	}

	m := &specgen.Config.Mounts[i]
	m.Options = slices.DeleteFunc(m.Options, func(option string) bool {
		return option == "ro" || option == "rw"
	})
	m.Options = append(m.Options, accessOption)
}
