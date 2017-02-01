## oci-storage-changes 1 "August 2016"

## NAME
oci-storage changes - Produce a list of changes in a layer

## SYNOPSIS
**oci-storage** **changes** *layerNameOrID* [*referenceLayerNameOrID*]

## DESCRIPTION
When a layer is first created, it contains no changes relative to its parent
layer.  After that is changed, the *oci-storage changes* command can be used to
obtain a summary of which files have been added, deleted, or modified in the
layer.

## EXAMPLE
**oci-storage changes f3be6c6134d0d980936b4c894f1613b69a62b79588fdeda744d0be3693bde8ec**

## SEE ALSO
oci-storage-applydiff(1)
oci-storage-diff(1)
oci-storage-diffsize(1)
