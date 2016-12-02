% kpod(8) # kpod-launch - Simple management tool for pods and images
% Dan Walsh
% SEPTEMBER 2016
# NAME
kpod-launch - Launch a new pod

# SYNOPSIS
**kpod launch**
[**--help**|**-h**]

# DESCRIPTION
Launch a container process in a new pod. **kpod launch** starts a process with
its own file system, its own networking, and its own isolated process tree.
The IMAGE which starts the process may define defaults related to the process
that will be launch in the pod, the networking to expose, and more, but
**kpod launch** gives final control to the operator or administrator who
starts the pod from the image. For that reason **kpod launch** has more
options than any other kpod commands.

If the IMAGE is not already loaded then **kpod launch** will pull the IMAGE, and
all image dependencies, from the repository in the same way launching **kpod
pull** IMAGE, before it starts the container from that image.

**kpod [GLOBAL OPTIONS]**

**kpod [GLOBAL OPTIONS] launch [OPTIONS]**

# GLOBAL OPTIONS

**--help, -h**
  Print usage statement

# COMMANDS

## launch
Launch a pod

# SEE ALSO
kpod(1), ocid(8), ocid.conf(5)

# HISTORY
Dec 2016, Originally compiled by Dan Walsh <dwalsh@redhat.com>
