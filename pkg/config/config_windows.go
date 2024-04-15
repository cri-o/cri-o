package config

import "github.com/cri-o/cri-o/utils/errdefs"

// Defaults for linux/unix if none are specified
const (
	cniConfigDir             = "C:\\cni\\etc\\net.d\\"
	cniBinDir                = "C:\\cni\\bin\\"
	containerExitsDir        = "C:\\crio\\run\\exits\\"
	ContainerAttachSocketDir = "C:\\crio\\run\\"

	// CrioConfigPath is the default location for the conf file
	CrioConfigPath = "C:\\crio\\etc\\crio.conf"

	// CrioConfigDropInPath is the default location for the drop-in config files
	CrioConfigDropInPath = "C:\\crio\\etc\\crio.conf.d"

	// CrioSocketPath is where the unix socket is located
	CrioSocketPath = "C:\\crio\\run\\crio.sock"

	// CrioVersionPath is where the CRI-O version file is located
	CrioConfigPath = "C:\\crio\\etc\\version"
)

// checkKernelRROMountSupport checks the kernel support for the Recursive Read-only (RRO) mounts.
func checkKernelRROMountSupport() error {
	return errdefs.ErrNotImplemented
}
