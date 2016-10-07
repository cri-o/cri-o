% ocid(8) Open Container Initiative Daemon
% Dan Walsh
% SEPTEMBER 2016
# NAME
ocid - Enable OCI Kubernetes Container Runtime daemon

# SYNOPSIS
**ocid**
[**--conmon**=[*value*]]
[**--containerdir**=[*value*]]
[**--debug**]
[**--help**|**-h**]
[**--log**=[*value*]]
[**--log-format value**]
[**--pause**=[*value*]]
[**--root**=[*value*]]
[**--runtime**=[*value*]]
[**--sandboxdir**=[*value*]]
[**--selinux-enabled**]
[**--socket**=[*value*]]
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

**--conmon**=""
  path to the conmon executable (default: "/usr/libexec/ocid/conmon")

**--containerdir**=""
  OCID container dir (default: "/var/lib/ocid/containers")

**--debug**
  Enable debug output for logging

**--help, -h**
  Print usage statement

**--log**=""
  Set the log file path where internal debug information is written

**--log-format**=""
  Set the format used by logs ('text' (default), or 'json') (default: "text")

**--pause**=""
  Path to the pause executable (default: "/usr/libexec/ocid/pause")

**--root**=""
  OCID root dir (default: "/var/lib/ocid")

**--runtime**=""
  OCI runtime path (default: "/usr/bin/runc")

**--sandboxdir**=""
  OCID pod sandbox dir (default: "/var/lib/ocid/sandboxes")

**--selinux-enabled**
  Enable selinux support

**--socket**=""
  Path to ocid socket (default: "/var/run/ocid.sock")

**--version, -v**
  Print the version

# HISTORY
Sept 2016, Originally compiled by Dan Walsh <dwalsh@redhat.com>
