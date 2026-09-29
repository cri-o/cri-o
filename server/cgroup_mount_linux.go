package server

import (
	"errors"
	"fmt"
	"slices"

	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/config/node"
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
	case types.CgroupMountMode_CGROUP_MOUNT_MODE_READ_WRITE:
		return checkWritableCgroupsSupported()
	default:
		return fmt.Errorf("unsupported cgroup mount mode %q", mode)
	}
}

func checkWritableCgroupsSupported() error {
	if !node.CgroupIsV2() {
		return errors.New("writable cgroups require cgroup v2")
	}

	hasNsdelegate, err := node.CgroupHasNsdelegate()
	if err != nil {
		return fmt.Errorf("check cgroup nsdelegate mount option: %w", err)
	}

	if !hasNsdelegate {
		return errors.New(
			"writable cgroups require /sys/fs/cgroup to be mounted with nsdelegate",
		)
	}

	return nil
}

func applyCgroupMountMode(specgen *generate.Generator, config *types.ContainerConfig) {
	var accessOption string

	switch config.GetLinux().GetSecurityContext().GetCgroupMountMode() {
	case types.CgroupMountMode_CGROUP_MOUNT_MODE_READ_ONLY:
		accessOption = "ro"
	case types.CgroupMountMode_CGROUP_MOUNT_MODE_READ_WRITE:
		accessOption = "rw"
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
