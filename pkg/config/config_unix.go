// +build !windows

package config

// Defaults for linux/unix if none are specified
const (
	cniConfigDir             = "/etc/cni/net.d/"
	cniBinDir                = "/opt/cni/bin/"
	containerExitsDir        = "/var/run/crio/exits"
	ContainerAttachSocketDir = "/var/run/crio"

	// CrioConfigPath is the default location for the conf file
	CrioConfigPath = "/etc/crio/crio.conf"

	// CrioConfigDropInPath is the default location for the drop-in config files
	CrioConfigDropInPath = "/etc/crio/crio.conf.d"

	// CrioSocketPath is where the unix socket is located
	CrioSocketPath = "/var/run/crio/crio.sock"

	// CrioPidPath is where the CRI-O pid file is located
	CrioPidPath = "/var/run/crio/pid"

	// CrioVersionPath is where the CRI-O version file is located
	CrioVersionPath = "/var/run/crio/version"
)
