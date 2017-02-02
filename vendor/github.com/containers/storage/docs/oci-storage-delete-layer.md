## oci-storage-delete-layer 1 "August 2016"

## NAME
oci-storage delete-layer - Delete a layer

## SYNOPSIS
**oci-storage** **delete-layer** *layerNameOrID*

## DESCRIPTION
Deletes a layer if it is not currently being used by any images or containers,
and is not the parent of any other layers.

## EXAMPLE
**oci-storage delete-layer my-base-layer**

## SEE ALSO
oci-storage-create-layer(1)
oci-storage-delete-image(1)
oci-storage-delete-layer(1)
