## kpod-push "1" "June 2017" "kpod"

## NAME
kpod push - push an image to a specified location

## SYNOPSIS
**kpod** **push** [*options* [...]] **imageID [...]** **TRANSPORT:REFERENCE**

## DESCRIPTION
Pushes an image to a specified location

## OPTIONS

**disable-compression, D**
  Don't compress layers

**signature-policy**=""
  Pathname of signature policy file (not usually used)

**creds**=""
  Credentials (USERNAME:PASSWORD) to use for authenticating to a registry

**cert-dir**=""
  Pathname of a directory containing TLS  certificates and keys

**tls-verify**=[true|false]
  Require HTTPS and verify certificates when contacting registries (default: true)

**remove-signatures**
  Discard any pre-existing signatures in the image

**sign-by**=""
  Add a signature at the destination using the specified key

**quiet, q**
  Don't output progress information when pushing images

## EXAMPLE

kpod push fedora:25 containers-storage:[overlay2@/var/lib/containers/storage]fedora

kpod push --disable-compression busybox:latest dir:/tmp/busybox

kpod push --creds=myusername:password123 redis:alpine docker://myusername/redis:alpine

## SEE ALSO
kpod(1)
