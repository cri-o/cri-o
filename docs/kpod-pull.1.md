% kpod(8) # kpod-pull - Simple tool to pull an image from a registry
% Urvashi Mohnani
% JULY 2017
# NAME
kpod-pull - Pull an image from a registry

# SYNOPSIS
**kpod pull**
**NAME[:TAG|@DISGEST]**
[**--help**|**-h**]

# DESCRIPTION
Copies an image from a registry onto the local machine. **kpod pull** pulls an
image from Docker Hub if a registry is not specified in the command line argument.
If an image tag is not specified, **kpod pull** defaults to the image with the
**latest** tag (if it exists) and pulls it. **kpod pull** can also pull an image
using its digest **kpod pull [image]@[digest]**.

**kpod [GLOBAL OPTIONS]**

**kpod pull [GLOBAL OPTIONS]**

**kpod pull NAME[:TAG|@DIGEST] [GLOBAL OPTIONS]**

# GLOBAL OPTIONS

**--help, -h**
  Print usage statement

# COMMANDS

## pull
Pull an image from a registry

# SEE ALSO
kpod(1), crio(8), crio.conf(5)

# HISTORY
July 2017, Originally compiled by Urvashi Mohnani <umohnani@redhat.com>
