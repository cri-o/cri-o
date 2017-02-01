## oci-storage-unmount 1 "August 2016"

## NAME
oci-storage unmount - Unmount a layer or a container's layer

## SYNOPSIS
**oci-storage** **unmount** *layerOrContainerMountpointOrNameOrID*

## DESCRIPTION
Unmounts a layer or a container's layer from the host's filesystem.

## EXAMPLE
**oci-storage unmount my-container**
**oci-storage unmount /var/lib/oci-storage/mounts/my-container**

## SEE ALSO
oci-storage-mount(1)
