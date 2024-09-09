package config

import "github.com/cri-o/cri-o/utils/errdefs"

// Defaults if none are specified
// Defaults for linux/unix if none are specified
const (
	cniConfigDir             = "/usr/local/etc/cni/net.d/"
	cniBinDir                = "/usr/local/libexec/cni/"
	containerExitsDir        = "/var/run/crio/exits"
	ContainerAttachSocketDir = "/var/run/crio"

	// CrioConfigPath is the default location for the conf file
	CrioConfigPath = "/usr/local/etc/crio/crio.conf"

	// CrioConfigDropInPath is the default location for the drop-in config files
	CrioConfigDropInPath = "/usr/local/etc/crio/crio.conf.d"

	// CrioSocketPath is where the unix socket is located
	CrioSocketPath = "/var/run/crio/crio.sock"

	// CrioVersionPathTmp is where the CRI-O version file is located on a tmpfs disk
	// used to check if we should wipe containers
	CrioVersionPathTmp = "/var/run/crio/version"

	// CrioCleanShutdownFile is the location CRI-O will lay down the clean shutdown file
	// that checks whether we've had time to sync before shutting down.
	// If not, crio wipe will clear the storage directory.
	CrioCleanShutdownFile = "/var/db/crio/clean.shutdown"

	DefaultRuntime       = "ocijail"
	DefaultRuntimeType   = "oci"
	DefaultRuntimeRoot   = "/var/run/ocijail"
	defaultMonitorCgroup = ""

	// ImageVolumesBind option is for using bind mounted volumes
	ImageVolumesBind ImageVolumesType = "nullfs"
	// DefaultPauseImage is default pause image
	DefaultPauseImage string = "quay.io/dougrabson/pause:latest"
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
