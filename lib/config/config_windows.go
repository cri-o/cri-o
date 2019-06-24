// +build windows

package config

import "github.com/cri-o/cri-o/oci"

// Defaults for linux/unix if none are specified
const (
	cniConfigDir             = "C:\\cni\\etc\\net.d\\"
	cniBinDir                = "C:\\cni\\bin\\"
	lockPath                 = "C:\\crio\\run\\crio.lock"
	containerExitsDir        = "C:\\crio\\run\\exits\\"
	ContainerAttachSocketDir = "C:\\crio\\run\\"
)
