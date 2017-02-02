## oci-storage-get-container-run-dir 1 "Sepember 2016"

## NAME
oci-storage get-container-run-dir - Find runtime lookaside directory for a container

## SYNOPSIS
**oci-storage** **get-container-run-dir** [*options* [...]] *containerNameOrID*

## DESCRIPTION
Prints the location of a directory which the caller can use to store lookaside
information which should be cleaned up when the host is rebooted.

## EXAMPLE
**oci-storage get-container-run-dir my-container**

## SEE ALSO
oci-storage-get-container-dir(1)
