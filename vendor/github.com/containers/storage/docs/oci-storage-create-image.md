## oci-storage-create-image 1 "August 2016"

## NAME
oci-storage create-image - Create an image

## SYNOPSIS
**oci-storage** **create-image** [*options*...] *topLayerNameOrID*

## DESCRIPTION
Creates an image, referring to the specified layer as the one which should be
used as the basis for containers which will be based on the image.

## OPTIONS
**-n | --name** *name*

Sets an optional name for the image.  If a name is already in use, an error is
returned.

**-i | --id** *ID*

Sets the ID for the image.  If none is specified, one is generated.

**-m | --metadata** *metadata-value*

Sets the metadata for the image to the specified value.

**-f | --metadata-file** *metadata-file*

Sets the metadata for the image to the contents of the specified file.

## EXAMPLE
**oci-storage create-image -f manifest.json -n new-image somelayer**

## SEE ALSO
oci-storage-create-container(1)
oci-storage-create-layer(1)
oci-storage-delete-image(1)
