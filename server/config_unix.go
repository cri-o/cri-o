// +build !windows

package server

// CrioConfigPath is the default location for the conf file
const CrioConfigPath = "/etc/crio/crio.conf"

// CrioSocketPath is where the unix socket is located
const CrioSocketPath = "/var/run/crio/crio.sock"

// CrioVersionPath is where the CRI-O version file is located
const CrioVersionPath = "/var/lib/crio/version"
