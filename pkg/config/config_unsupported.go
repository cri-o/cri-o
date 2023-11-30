//go:build !linux && !freebsd

package config

// Defaults if none are specified
// This uses the Linux values, just to have something that compiles. They don’t even pass unit tests.
const (
	defaultRuntime       = "invalid defaultRuntime"
	DefaultRuntimeType   = "invalid DefaultRuntimeType"
	DefaultRuntimeRoot   = "invalid DefaultRuntimeRoot"
	defaultMonitorCgroup = "invalid defaultMonitorCgroup"
	// ImageVolumesBind option is for using bind mounted volumes
	ImageVolumesBind ImageVolumesType = "invalid ImageVolumesBind"
	// DefaultPauseImage is default pause image
	DefaultPauseImage string = "registry.k8s.io/pause:3.9"
)

func selinuxEnabled() bool {
	return false
}
