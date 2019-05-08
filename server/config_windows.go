// +build windows

package server

//CrioConfigPath is the default location for the conf file
var CrioConfigPath = "C:\\crio\\etc\\crio.conf"

// CrioSocketPath is where the unix socket is located
const CrioSocketPath = "C:\\crio\\run\\crio.sock"

// CrioVersionPath is where the CRI-O version file is located
var CrioConfigPath = "C:\\crio\\etc\\version"
