// +build windows

package lib

import "github.com/cri-o/cri-o/oci"

// Defaults for linux/unix if none are specified
const (
	conmonPath         = "C:\\crio\\bin\\conmon"
	seccompProfilePath = "C:\\crio\\etc\\seccomp.json"
	cniConfigDir       = "C:\\cni\\etc\\net.d\\"
	cniBinDir          = "C:\\cni\\bin\\"
	lockPath           = "C:\\crio\\run\\crio.lock"
	containerExitsDir  = oci.ContainerExitsDir
	crioConfigPath     = "C:\\crio\\etc\\version"
)
