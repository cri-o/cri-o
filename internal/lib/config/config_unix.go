// +build !windows

package config

// Defaults for linux/unix if none are specified
const (
	cniConfigDir             = "/etc/cni/net.d/"
	cniBinDir                = "/opt/cni/bin/"
	lockPath                 = "/run/crio.lock"
	containerExitsDir        = "/var/run/crio/exits"
	ContainerAttachSocketDir = "/var/run/crio"
)
