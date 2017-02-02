## oci-storage-exists 1 "August 2016"

## NAME
oci-storage exists - Check if a layer, image, or container exists

## SYNOPSIS
**oci-storage** **exists** [*options* [...]] *layerOrImageOrContainerNameOrID* [...]

## DESCRIPTION
Checks if there are layers, images, or containers which have the specified
names or IDs.

## OPTIONS
**-c | --container**

Only succeed if the names or IDs are that of containers.

**-i | --image**

Only succeed if the names or IDs are that of images.

**-l | --layer**

Only succeed if the names or IDs are that of layers.

**-q | --quiet**

Suppress output.

## EXAMPLE
**oci-storage exists my-base-layer**
