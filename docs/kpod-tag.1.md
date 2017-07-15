## kpod-tag "1" "June 2017" "kpod"

# NAME
kpod tag - add tags to an image

# SYNOPSIS
**kpod tag**
[**--help**|**-h**]

# DESCRIPTION
Assigns a new alias to an image in a registry.  An alias refers to the entire image name, including the optional **TAG** after the ':'

**kpod [GLOBAL OPTIONS]**

**kpod [GLOBAL OPTIONS] tag [OPTIONS]**

# GLOBAL OPTIONS

**--help, -h**
  Print usage statement

# EXAMPLES

  kpod tag 0e3bbc2 fedora:latest

  kpod tag httpd myregistryhost:5000/fedora/httpd:v2

# SEE ALSO
kpod(1), crio(8), crio.conf(5)
