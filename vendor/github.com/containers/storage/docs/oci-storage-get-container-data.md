## oci-storage-get-container-data 1 "August 2016"

## NAME
oci-storage get-container-data - Retrieve lookaside data for a container

## SYNOPSIS
**oci-storage** **get-container-data** [*options* [...]] *containerNameOrID* *dataName*

## DESCRIPTION
Retrieves a piece of named data which is associated with a container.

## OPTIONS
**-f | --file** *file*

Write the data to a file instead of stdout.

## EXAMPLE
**oci-storage get-container-data -f config.json my-container configuration**

## SEE ALSO
oci-storage-list-container-data(1)
oci-storage-set-container-data(1)
