//go:build !windows && !freebsd

package config

// Defaults for linux/unix if none are specified.
const (
	cniConfigDir             = "/etc/cni/net.d/"
	cniBinDir                = "/opt/cni/bin/"
	containerExitsDir        = "/var/run/crio/exits"
	ContainerAttachSocketDir = "/var/run/crio"

	// CrioConfigPath is the default location for the conf file.
	CrioConfigPath = "/etc/crio/crio.conf"

	// CrioConfigDropInPath is the default location for the drop-in config files.
	CrioConfigDropInPath = "/etc/crio/crio.conf.d"

	// CrioSocketPath is where the unix socket is located.
	CrioSocketPath = "/var/run/crio/crio.sock"

	// CrioVersionPathTmp is where the CRI-O version file is located on a tmpfs disk
	// used to check if we should wipe containers.
	CrioVersionPathTmp = "/var/run/crio/version"

	// CrioCleanShutdownFile is the location CRI-O will lay down the clean shutdown file
	// that checks whether we've had time to sync before shutting down.
	// If not, crio wipe will clear the storage directory.
	CrioCleanShutdownFile = "/var/lib/crio/clean.shutdown"
)
