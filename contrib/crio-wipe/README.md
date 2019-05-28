# crio-wipe

crio-wipe is a program that reads CRI-O's version file, and compares it to the output of crio --version.
If the version is deemed old enough, crio-wipe wipes container storage on the node.
If there is no version file present, or the version file is malformatted, crio-wipe also will wipe the storage.


crio-wipe returns 0 on success and 1 on error. If a wipe happened, it will be noted in stdout. Otherwise, crio-wipe will not wipe silently


crio-wipe by default assumes:
* the containers storage directory is `/var/lib/containers`
* the location of the version file is `/var/lib/crio/version`
* formatting of `crio --version` resembles `crio version $MAJOR.$MINOR...`
* formatting of version file resembles `"$MAJOR.$MINOR...`

The latter two formatting assumptions can only be changed by changing crio-wipe

Users have access to these flags to change crio-wipe's behavior:

| Flag         | Usage                                                                 |
|--------------|-----------------------------------------------------------------------|
| -d [value]   | Change the location of the storage dir to be wiped                    |
| -f [value]   | Change the location of the version file to be read as the old version |
| -w [integer] | Non-zero values tell crio-wipe to not actually remove the storage-dir |

crio-wipe has a test suite that can be run with `bats test.bats`
