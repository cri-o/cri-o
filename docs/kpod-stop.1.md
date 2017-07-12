## kpod-stop"1" "July 2017" "kpod"

## NAME
kpod stop - Stops one or more containers

## SYNOPSIS
**kpod** **stop** **containerID[...]**

## DESCRIPTION
Stops one or more containers.

## OPTIONS

**--timeout, -t**

Seconds to wait to kill the container after a graceful stop is requested.  The default is 10.

**--config**

Path to an alternative configuration file.  The default path is */etc/crio/crio.conf*.

## EXAMPLE

kpod stop containerID 

kpod stop containerID1 containerID2

kpod stop -t 3 containerID

kpod stop --config ~/crio.conf containerID

## SEE ALSO
kpod(1)
