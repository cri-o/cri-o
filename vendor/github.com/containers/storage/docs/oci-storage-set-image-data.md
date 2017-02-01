## oci-storage-set-image-data 1 "August 2016"

## NAME
oci-storage set-image-data - Set lookaside data for an image

## SYNOPSIS
**oci-storage** **set-image-data** [*options* [...]] *imageNameOrID* *dataName*

## DESCRIPTION
Sets a piece of named data which is associated with an image.

## OPTIONS
**-f | --file** *filename*

Read the data contents from a file instead of stdin.

## EXAMPLE
**oci-storage set-image-data -f ./manifest.json my-image manifest**

## SEE ALSO
oci-storage-get-image-data(1)
oci-storage-list-image-data(1)
