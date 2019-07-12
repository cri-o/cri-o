// +build !windows

package config

// Defaults for linux/unix if none are specified
const (
	cniConfigDir             = "/etc/cni/net.d/"
	cniBinDir                = "/opt/cni/bin/"
	lockPath                 = "/run/crio.lock"
	containerExitsDir        = "/var/run/crio/exits"
	ContainerAttachSocketDir = "/var/run/crio"

	// CrioConfigPath is the default location for the conf file
	CrioConfigPath = "/etc/crio/crio.conf"

	// CrioSocketPath is where the unix socket is located
	CrioSocketPath = "/var/run/crio/crio.sock"

	// CrioVersionPath is where the CRI-O version file is located
	CrioVersionPath = "/var/lib/crio/version"
)
