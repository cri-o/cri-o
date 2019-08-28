// +build !windows

package lib

import "github.com/cri-o/cri-o/oci"

// Defaults for linux/unix if none are specified
const (
	conmonPath         = "/usr/local/libexec/crio/conmon"
	seccompProfilePath = "/etc/crio/seccomp.json"
	cniConfigDir       = "/etc/cni/net.d/"
	cniBinDir          = "/opt/cni/bin/"
	lockPath           = "/run/crio.lock"
	containerExitsDir  = oci.ContainerExitsDir
	crioVersionPath    = "/var/lib/crio/version"
)
