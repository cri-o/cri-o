## oci-storage-create-layer 1 "August 2016"

## NAME
oci-storage create-layer - Create a layer

## SYNOPSIS
**oci-storage** **create-layer** [*options* [...]] [*parentLayerNameOrID*]

## DESCRIPTION
Creates a new layer which either has a specified layer as its parent, or if no
parent is specified, is empty.

## OPTIONS
**-n** *name*

Sets an optional name for the layer.  If a name is already in use, an error is
returned.

**-i | --id** *ID*

Sets the ID for the layer.  If none is specified, one is generated.

**-m | --metadata** *metadata-value*

Sets the metadata for the layer to the specified value.

**-f | --metadata-file** *metadata-file*

Sets the metadata for the layer to the contents of the specified file.

**-l | --label** *mount-label*

Sets the label which should be assigned as an SELinux context when mounting the
layer.

## EXAMPLE
**oci-storage create-layer -f manifest.json -n new-layer somelayer**

## SEE ALSO
oci-storage-create-container(1)
oci-storage-create-image(1)
oci-storage-delete-layer(1)
