//go:build !linux && !freebsd && !windows
// +build !linux,!freebsd,!windows

package config

import "github.com/cri-o/cri-o/utils/errdefs"

// Defaults if none are specified
// This uses the Linux values, just to have something that compiles. They don’t even pass unit tests.
const (
	DefaultRuntime       = "invalid DefaultRuntime"
	DefaultRuntimeType   = "invalid DefaultRuntimeType"
	DefaultRuntimeRoot   = "invalid DefaultRuntimeRoot"
	defaultMonitorCgroup = "invalid defaultMonitorCgroup"
	// ImageVolumesBind option is for using bind mounted volumes
	ImageVolumesBind ImageVolumesType = "invalid ImageVolumesBind"
	// DefaultPauseImage is default pause image
	DefaultPauseImage string = "registry.k8s.io/pause:3.10"
)

func selinuxEnabled() bool {
	return false
}

// checkKernelRROMountSupport checks the kernel support for the Recursive Read-only (RRO) mounts.
func checkKernelRROMountSupport() error {
	return errdefs.ErrNotImplemented
}

func (c *RuntimeConfig) ValidatePinnsPath(executable string) error {
	return nil
}
