//go:build !linux

package server

import (
	"fmt"
	"runtime"

	"github.com/opencontainers/runtime-tools/generate"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

const cgroupMountModeSupported = false

func validateCgroupMountMode(config *types.ContainerConfig) error {
	if mode := config.GetLinux().GetSecurityContext().GetCgroupMountMode(); mode != types.CgroupMountMode_CGROUP_MOUNT_MODE_UNSPECIFIED {
		return fmt.Errorf("unsupported cgroup mount mode %q on %s", mode, runtime.GOOS)
	}

	return nil
}

func applyCgroupMountMode(_ *generate.Generator, _ *types.ContainerConfig) {}
