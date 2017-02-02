## oci-storage-get-container-dir 1 "Sepember 2016"

## NAME
oci-storage get-container-dir - Find lookaside directory for a container

## SYNOPSIS
**oci-storage** **get-container-dir** [*options* [...]] *containerNameOrID*

## DESCRIPTION
Prints the location of a directory which the caller can use to store lookaside
information which should be cleaned up when the container is deleted.

## EXAMPLE
**oci-storage get-container-dir my-container**

## SEE ALSO
oci-storage-get-container-run-dir(1)
