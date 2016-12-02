% kpod(8) # kpod - Simple management tool for pods and images
% Dan Walsh
% SEPTEMBER 2016
# NAME
kpod

# SYNOPSIS
**kpod**
[**--help**|**-h**]

# DESCRIPTION
kpod is a simple client only tool to help with debugging issues when daemons
such as CRI runtime and the kubelet are not responding or failing. A shared API
layer could be created to share code between the daemon and kpod. kpod does not
require any daemon running. kpod utilizes the same underlying components that
ocid uses i.e. containers/image, container/storage, oci-runtime-tool/generate,
runc or any other OCI compatible runtime. kpod shares state with ocid and so
has the capability to debug pods/images created by ocid.

**kpod [GLOBAL OPTIONS]**

# GLOBAL OPTIONS

**--help, -h**
  Print usage statement

**--version, -v**
  Print the version

# COMMANDS

## launch
Launch a pod

# SEE ALSO
ocid(8), ocid.conf(5)

# HISTORY
Dec 2016, Originally compiled by Dan Walsh <dwalsh@redhat.com>
