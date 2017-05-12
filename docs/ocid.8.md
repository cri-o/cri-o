% ocid(8) Open Container Initiative Daemon
% Dan Walsh
% SEPTEMBER 2016
# NAME
ocid - Enable OCI Kubernetes Container Runtime daemon

# SYNOPSIS
**ocid**
[**--config**=[*value*]]
[**--conmon**=[*value*]]
[**--debug**]
[**--default-transport**=[*value*]]
[**--help**|**-h**]
[**--listen**=[*value*]]
[**--log**=[*value*]]
[**--log-format value**]
[**--pause-command**=[*value*]]
[**--pause-image**=[*value*]]
[**--root**=[*value*]]
[**--runroot**=[*value*]]
[**--runtime**=[*value*]]
[**--signature-policy**=[*value*]]
[**--storage-driver**=[*value*]]
[**--storage-opt**=[*value*]]
[**--selinux**]
[**--seccomp-profile**=[*value*]]
[**--apparmor-profile**=[*value*]]
[**---cni-config-dir**=[*value*]]
[**---cni-plugin-dir**=[*value*]]
[**--version**|**-v**]

# DESCRIPTION
OCI-based implementation of Kubernetes Container Runtime Interface Daemon

ocid is meant to provide an integration path between OCI conformant runtimes and the kubelet. Specifically, it implements the Kubelet Container Runtime Interface (CRI) using OCI conformant runtimes. The scope of ocid is tied to the scope of the CRI.

	* Support multiple image formats including the existing Docker image format
	* Support for multiple means to download images including trust & image verification
	* Container image management (managing image layers, overlay filesystems, etc)
	* Container process lifecycle management
	* Monitoring and logging required to satisfy the CRI
	* Resource isolation as required by the CRI

**ocid [GLOBAL OPTIONS]**

**ocid [GLOBAL OPTIONS] config [OPTIONS]**

# GLOBAL OPTIONS

**--apparmor_profile**=""
  Name of the apparmor profile to be used as the runtime's default (default: "ocid-default")

**--config**=""
  path to configuration file

**--conmon**=""
  path to the conmon executable (default: "/usr/local/libexec/ocid/conmon")

**--debug**
  Enable debug output for logging

**--default-transport**
  A prefix to prepend to image names that can't be pulled as-is.

**--help, -h**
  Print usage statement

**--listen**=""
  Path to ocid socket (default: "/var/run/ocid.sock")

**--log**=""
  Set the log file path where internal debug information is written

**--log-format**=""
  Set the format used by logs ('text' (default), or 'json') (default: "text")

**--pause-command**=""
  Path to the pause executable in the pause image (default: "/pause")

**--pause-image**=""
  Image which contains the pause executable (default: "kubernetes/pause")

**--root**=""
  OCID root dir (default: "/var/lib/containers/storage")

**--runroot**=""
  OCID state dir (default: "/var/run/containers/storage")

**--runtime**=""
  OCI runtime path (default: "/usr/local/sbin/runc")

**--selinux**=*true*|*false*
  Enable selinux support (default: false)

**--seccomp-profile**=""
  Path to the seccomp json profile to be used as the runtime's default (default: "/etc/ocid/seccomp.json")

**--signature-policy**=""
  Path to the signature policy json file (default: "", to use the system-wide default)

**--storage-driver**
  OCI storage driver (default: "devicemapper")

**--storage-opt**
  OCI storage driver option (no default)

**--cni-config-dir**=""
  CNI configuration files directory (default: "/etc/cni/net.d/")

**--cni-plugin-dir**=""
  CNI plugin binaries directory (default: "/opt/cni/bin/")

**--cpu-profile**
  Set the CPU profile file path

**--version, -v**
  Print the version

# COMMANDS
OCID's default command is to start the daemon. However, it currently offers a
single additional subcommand.

## config

Outputs a commented version of the configuration file that would've been used
by OCID. This allows you to save you current configuration setup and then load
it later with **--config**. Global options will modify the output.

**--default**
  Output the default configuration (without taking into account any configuration options).

# SEE ALSO
ocid.conf(5)

# HISTORY
Sept 2016, Originally compiled by Dan Walsh <dwalsh@redhat.com> and Aleksa Sarai <asarai@suse.de>
