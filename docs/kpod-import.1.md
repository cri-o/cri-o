% kpod(1) kpod-import - Simple tool to import a tarball as an image
% Urvashi Mohnani
# kpod-import "1" "November 2017" "kpod"

## NAME
kpod-import - import a tarball and save it as a filesystem image

## SYNOPSIS
**kpod import**
**TARBALL**
[**--change**|**-c**]
[**--message**|**-m**]
[**--help**|**-h**]

## DESCRIPTION
**kpod import** imports a tarball and saves it as a filesystem image.
The image configuration can be modified with the **--change** flag and
a commit message can be set using the **--message** flag.

**kpod [GLOBAL OPTIONS]**

**kpod import [GLOBAL OPTIONS]**

**kpod import [OPTIONS] CONTAINER**

## OPTIONS

**--change, -c**
Apply imgspecv1 configurations to the created image
Possible configurations include:
**USER** | **EXPOSE** | **ENV** | **ENTRYPOINT** | **CMD** | **VOLUME** | **WORKDIR** | **LABEL** | **STOPSIGNAL**

**--message, -m**
Set commit message for image imported

## EXAMPLES

```
# kpod import --change "CMD=/bin/bash ENTRYPOINT=/bin/sh LABEL=blue=image" ctr.tar image-imported
Getting image source signatures
Copying blob sha256:b41deda5a2feb1f03a5c1bb38c598cbc12c9ccd675f438edc6acd815f7585b86
 25.80 MB / 25.80 MB [======================================================] 0s
Copying config sha256:c16a6d30f3782288ec4e7521c754acc29d37155629cb39149756f486dae2d4cd
 448 B / 448 B [============================================================] 0s
Writing manifest to image destination
Storing signatures
```

```
# cat ctr.tar | kpod import --message "importing the ctr.tar tarball" - image-imported
Getting image source signatures
Copying blob sha256:b41deda5a2feb1f03a5c1bb38c598cbc12c9ccd675f438edc6acd815f7585b86
 25.80 MB / 25.80 MB [======================================================] 0s
Copying config sha256:af376cdda5c0ac1d9592bf56567253d203f8de6a8edf356c683a645d75221540
 376 B / 376 B [============================================================] 0s
Writing manifest to image destination
Storing signatures
```

```
# cat ctr.tar | kpod import -
Getting image source signatures
Copying blob sha256:b41deda5a2feb1f03a5c1bb38c598cbc12c9ccd675f438edc6acd815f7585b86
 25.80 MB / 25.80 MB [======================================================] 0s
Copying config sha256:d61387b4d5edf65edee5353e2340783703074ffeaaac529cde97a8357eea7645
 378 B / 378 B [============================================================] 0s
Writing manifest to image destination
Storing signatures
```

```
kpod import http://lacrosse.redhat.com/~umohnani/ctr.tar url-image
Downloading from "http://lacrosse.redhat.com/~umohnani/ctr.tar"
Getting image source signatures
Copying blob sha256:b41deda5a2feb1f03a5c1bb38c598cbc12c9ccd675f438edc6acd815f7585b86
 25.80 MB / 25.80 MB [======================================================] 0s
Copying config sha256:5813fe8a3b18696089fd09957a12e88bda43dc1745b5240879ffffe93240d29a
 419 B / 419 B [============================================================] 0s
Writing manifest to image destination
Storing signatures
```

## SEE ALSO
kpod(1), kpod-export(1), crio(8), crio.conf(5)

## HISTORY
November 2017, Originally compiled by Urvashi Mohnani <umohnani@redhat.com>
