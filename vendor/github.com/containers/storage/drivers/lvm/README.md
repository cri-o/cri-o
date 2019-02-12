lvm
===

This driver uses an LVM thin pool to create a block device for each layer, with
each layer initially containing the contents of either its parent layer or a
freshly-formatted filesystem.

Options
=======

* lvm.vg - The name of the volume group containing the thin pool.  If it is not
  active when the driver is loaded, the driver will use a loopback device as a
  physical volume and create the volume group using it.  The default volume
  group name is "containers".
* lvm.pool - The name of the thin pool volume in the volume group.  If the thin
  pool does not exist, the driver will create a data volume and metadata
  volume, and then tie them together into a thin pool with the desired name.
  The default pool name is "loopbackpool".
* lvm.fs - The type of filesystem to place on layers which are created without
  a parent layer.  The default is "xfs".
* lvm.loopback - The name of the loopback file to use when creating the volume
  group using a loopback device.  If the file already exists, it is not
  modified.  Its name can be either an absolute path or a path relative to the
  storage root directory's "lvm" subdirectory.  The default is "loopbackfile".
* lvm.loopbacksize - The size of the loopback file to create, if one needs to
  be created.  The default is 10GB.
* lvm.sparse - Whether or not the loopback file, if one needs to be created,
  will be a sparse file.

Internals
=========

At startup:
* If the specified volume group doesn't exist:
  * create a loopback file
  * attach the loopback file as a loopback device
  * use "lvm pvcreate" to put a physical volume header on it
  * use "lvm vgcreate" to create the volume group using it
* If the specified pool doesn't exist:
  * create a volume in the group for plain data (a "ThinDataLV" in lvmthin(7)'s
    terms)
  * create a volume in the group for metadata (a "ThinMetaLV" in lvmthin(7)'s
    terms)
  * tie those two volumes together into a thin pool (a "ThinPoolLV" in
    lvmthin(7)'s terms)

Creating layers:
* To create a layer with no parent (i.e., a base layer):
  * create a new device (a "ThinLV" in lvmthin(7)'s terms)
  * format it with "mkfs", passing "-t" and the configured filesystem type
* To create a layer with a parent:
  * create a snapshot of the parent layer's device (a "SnapLV" in lvmthin(7)'s
    terms)
  * for "xfs" filesystems, run "xfs\_admin -U generate" to give the new device
    a new UUID
  * for "ext2"/"ext3"/"ext4dev"/"ext4" filesystems, run "tune2fs -U time" to
    give the new device a new UUID

Deleting layers:
* Delete the layer's logical volume.

Shutting down:
* Get a list of the physical volumes in the volume group.
* Attempt to deactivate the volume group.
* If the volume group is deactivated, get a list of the current loopback
  devices.
* For every device in the list of physical volumes from our volume group that
  is also in the list of loopback devices, attempt to detach the loopback
  device.
