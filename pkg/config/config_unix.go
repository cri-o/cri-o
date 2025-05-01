//go:build !windows && !freebsd

package config

// Defaults for linux/unix if none are specified.
const (
	cniConfigDir             = "/etc/cni/net.d/"
	cniBinDir                = "/opt/cni/bin/"
	containerExitsDir        = "/var/run/crio/exits"
	ContainerAttachSocketDir = "/var/run/crio"

	// CrioConfigPathEtc is the default location for the conf file.
	CrioConfigPathEtc = "/etc/crio/crio.conf"

	// CrioConfigPathUsr is the second location for the conf file.
	CrioConfigPathUsr = "/usr/lib/crio/crio.conf"

	// CrioConfigDropInPathEtc is the default location for the drop-in config files.
	CrioConfigDropInPathEtc = "/etc/crio/crio.conf.d"

	// CrioConfigDropInPathUsr is the second default location for the drop-in config files.
	CrioConfigDropInPathUsr = "/usr/lib/crio/crio.conf.d"

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
