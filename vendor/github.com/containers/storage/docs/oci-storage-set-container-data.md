## oci-storage-set-container-data 1 "August 2016"

## NAME
oci-storage set-container-data - Set lookaside data for a container

## SYNOPSIS
**oci-storage** **set-container-data** [*options* [...]] *containerNameOrID* *dataName*

## DESCRIPTION
Sets a piece of named data which is associated with a container.

## OPTIONS
**-f | --file** *filename*

Read the data contents from a file instead of stdin.

## EXAMPLE
**oci-storage set-container-data -f ./config.json my-container configuration**

## SEE ALSO
oci-storage-get-container-data(1)
oci-storage-list-container-data(1)
