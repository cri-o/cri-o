## oci-storage-create-container 1 "August 2016"

## NAME
oci-storage create-container - Create a container

## SYNOPSIS
**oci-storage** **create-container** [*options*...] *imageNameOrID*

## DESCRIPTION
Creates a container, using the specified image as the starting point for its
root filesystem.

## OPTIONS
**-n | --name** *name*

Sets an optional name for the container.  If a name is already in use, an error
is returned.

**-i | --id** *ID*

Sets the ID for the container.  If none is specified, one is generated.

**-m | --metadata** *metadata-value*

Sets the metadata for the container to the specified value.

**-f | --metadata-file** *metadata-file*

Sets the metadata for the container to the contents of the specified file.

## EXAMPLE
**oci-storage create-container -f manifest.json -n new-container goodimage**

## SEE ALSO
oci-storage-create-image(1)
oci-storage-create-layer(1)
oci-storage-delete-container(1)
