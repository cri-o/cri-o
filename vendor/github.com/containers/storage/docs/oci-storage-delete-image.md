## oci-storage-delete-image 1 "August 2016"

## NAME
oci-storage delete-image - Delete an image

## SYNOPSIS
**oci-storage** **delete-image** *imageNameOrID*

## DESCRIPTION
Deletes an image if it is not currently being used by any containers.  If the
image's top layer is not being used by any other images, it will be removed.
If that image's parent is then not being used by other images, it, too, will be
removed, and the this will be repeated for each parent's parent.

## EXAMPLE
**oci-storage delete-image my-base-image**

## SEE ALSO
oci-storage-create-image(1)
oci-storage-delete-container(1)
oci-storage-delete-layer(1)
