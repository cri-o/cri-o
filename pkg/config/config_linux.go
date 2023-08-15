package config

import selinux "github.com/opencontainers/selinux/go-selinux"

// Defaults if none are specified
const (
	defaultRuntime       = "runc"
	DefaultRuntimeType   = "oci"
	DefaultRuntimeRoot   = "/run/runc"
	defaultMonitorCgroup = "system.slice"
	// ImageVolumesBind option is for using bind mounted volumes
	ImageVolumesBind ImageVolumesType = "bind"
	// DefaultPauseImage is default pause image
	DefaultPauseImage string = "registry.k8s.io/pause:3.9"
)

func selinuxEnabled() bool {
	return selinux.GetEnabled()
}

func (c *RuntimeConfig) ValidatePinnsPath(executable string) error {
	var err error
	c.PinnsPath, err = validateExecutablePath(executable, c.PinnsPath)

	return err
}
