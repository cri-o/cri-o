## oci-storage-delete 1 "August 2016"

## NAME
oci-storage delete - Force deletion of a layer, image, or container

## SYNOPSIS
**oci-storage** **delete** *layerOrImageOrContainerNameOrID*

## DESCRIPTION
Deletes a specified layer, image, or container, with no safety checking.  This
can corrupt data, and may be removed.

## EXAMPLE
**oci-storage delete my-base-layer**

## SEE ALSO
oci-storage-delete-container(1)
oci-storage-delete-image(1)
oci-storage-delete-layer(1)
