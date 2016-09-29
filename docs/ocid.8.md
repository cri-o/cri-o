% ocid(8) Open Container Initiative Daemon
% Dan Walsh
% SEPTEMBER 2016
# NAME
ocid - Enable OCI Kubernetes Container Runtime daemon

# SYNOPSIS
**ocid**
[**--root**=[*value*]]
[**--conmon**=[*value*]]
[**--sandboxdir**=[*value*]]
[**--containerdir**=[*value*]]
[**--socket**=[*value*]]
[**--runtime**=[*value*]]
[**--debug**]
[**--log**=[*value*]]
[**--log-format value**]
[**--help**|**-h**]
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

**ocid [OPTIONS]**

# OPTIONS

**--root**=""
  OCID root dir (default: "/var/lib/ocid")

**--sandboxdir**=""
  OCID pod sandbox dir (default: "/var/lib/ocid/sandboxes")

**--conmon**=""
  path to the conmon executable (default: "/usr/libexec/ocid/conmon")

**--containerdir**=""
  OCID container dir (default: "/var/lib/ocid/containers")

**--socket**=""
  Path to ocid socket (default: "/var/run/ocid.sock")

**--runtime**=""
  OCI runtime path (default: "/usr/bin/runc")

**--debug**
  Enable debug output for logging

**--log**=""
  Set the log file path where internal debug information is written

**--log-format**=""
  Set the format used by logs ('text' (default), or 'json') (default: "text")

**--help, -h**
  Print usage statement

**--version, -v**
  Print the version

# HISTORY
Sept 2016, Originally compiled by Dan Walsh <dwalsh@redhat.com>
