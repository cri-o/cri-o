// +build !windows

package config

// Defaults for linux/unix if none are specified
const (
	conmonPath               = "/usr/local/libexec/crio/conmon"
	seccompProfilePath       = "/etc/crio/seccomp.json"
	cniConfigDir             = "/etc/cni/net.d/"
	cniBinDir                = "/opt/cni/bin/"
	lockPath                 = "/run/crio.lock"
	containerExitsDir        = "/var/run/crio/exits"
	ContainerAttachSocketDir = "/var/run/crio"
)
