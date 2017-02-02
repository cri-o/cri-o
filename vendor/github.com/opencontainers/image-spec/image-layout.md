## OCI Image Layout Specification

The OCI Image Layout is a slash separated layout of OCI content-addressable blobs and [location-addressable](https://en.wikipedia.org/wiki/Content-addressable_storage#Content-addressed_vs._location-addressed) references (refs).
This layout MAY be used in a variety of different transport mechanisms: archive formats (e.g. tar, zip), shared filesystem environments (e.g. nfs), or networked file fetching (e.g. http, ftp, rsync).

Given an image layout and a ref, a tool can create an [OCI Runtime Specification bundle](https://github.com/opencontainers/runtime-spec/blob/v1.0.0-rc2/bundle.md) by:

* Following the ref to find a [manifest](manifest.md#image-manifest), possibly via a [manifest list](manifest-list.md#manifest-list)
* [Applying the filesystem layers](layer.md#applying) in the specified order
* Converting the [image configuration](config.md) into an [OCI Runtime Specification `config.json`](https://github.com/opencontainers/runtime-spec/blob/v1.0.0-rc2/config.md)

# Content

The image layout MUST contain two top level directories:

- `blobs` contains content-addressable blobs.
  A blob has no schema and should be considered opaque.
- `refs` contains [descriptors][descriptors].
  Commonly pointing to an [image manifest](manifest.md#image-manifest) or an [image manifest list](manifest-list.md#oci-image-manifest-list-specification).

Both `blobs` and `refs` MAY be empty.

The image layout MUST also contain an `oci-layout` file:

- It MUST be a JSON object
- It MUST contain an `imageLayoutVersion` field
    - The `imageLayoutVersion` value will align with the OCI Image Specification version at the time changes to the layout are made, and will pin a given version until changes to the layout are required
- It MAY include additional fields

## Example Layout

This is an example image layout:

```
$ cd example.com/app/
$ find .
.
./blobs
./blobs/sha256/e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f
./blobs/sha256/afff3924849e458c5ef237db5f89539274d5e609db5db935ed3959c90f1f2d51
./blobs/sha256/9b97579de92b1c195b85bb42a11011378ee549b02d7fe9c17bf2a6b35d5cb079
./blobs/sha256/5b0bcabd1ed22e9fb1310cf6c2dec7cdef19f0ad69efa1f392e94a4333501270
./oci-layout
./refs
./refs/v1.0
./refs/stable-release
```

Blobs are named by their contents:

```
$ shasum -a 256 ./blobs/sha256/afff3924849e458c5ef237db5f89539274d5e609db5db935ed3959c90f1f2d51
afff3924849e458c5ef237db5f89539274d5e609db5db935ed3959c90f1f2d51 ./blobs/sha256/afff3924849e458c5ef237db5f89539274d5e609db5db935ed3959c90f1f2d51
```

## Refs

Object names in the `refs` subdirectories MUST NOT include characters outside of the set of "A" to "Z", "a" to "z", "0" to "9", the hyphen `-`, the dot `.`, and the underscore `_`.

No semantic restriction is given for object names in the `refs` subdirectory.
Each object in the `refs` subdirectory MUST be of type `application/vnd.oci.descriptor.v1+json`.
In general the `mediaType` of this [descriptor][descriptors] object will be either `application/vnd.oci.image.manifest.list.v1+json` or `application/vnd.oci.image.manifest.v1+json` although future versions of the spec MAY use a different mediatype.

**Implementor's Note:**
A common use case of refs is representing "tags" for a container image.
For example, an image may have a tag for different versions or builds of the software.
In the wild you often see "tags" like "v1.0.0-vendor.0", "2.0.0-debug", etc.
Those tags will often be represented in an image-layout repository with matching refs names like "v1.0.0-vendor.0", "2.0.0-debug", etc.

### Example Ref

This is an example `v1.0` ref with a manifest-list descriptor:

```
$ cat ./refs/v1.0 | jq
{
  "size": 4096,
  "digest": "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f",
  "mediaType": "application/vnd.oci.image.manifest.list.v1+json"
}
```

## Blobs

Object names in the `blobs` subdirectories are composed of a directory for each hash algorithm, the children of which will contain the actual content.
A blob, referenced with digest `<alg>:<hex>` (per [descriptor](descriptor.md#digests-and-verification)), MUST have its content stored in a file under `blobs/<alg>/<hex>`.
The character set of the entry name for `<hex>` and `<alg>` MUST match the respective grammar elements described in [descriptor](descriptor.md#digests-and-verification).
For example `sha256:5b` will map to the layout `blobs/sha256/5b`.

The blobs directory MAY contain blobs which are not referenced by any of the refs.

The blobs directory MAY be missing referenced blobs, in which case the missing blobs SHOULD be fulfilled by an external blob store.

### Example Blobs

```
$ cat ./blobs/sha256/9b97579de92b1c195b85bb42a11011378ee549b02d7fe9c17bf2a6b35d5cb079 | jq
{
  "schemaVersion": 2,
  "manifests": [
    {
      "mediaType": "application/vnd.oci.image.manifest.v1+json",
      "size": 7143,
      "digest": "sha256:afff3924849e458c5ef237db5f89539274d5e609db5db935ed3959c90f1f2d51",
      "platform": {
        "architecture": "ppc64le",
        "os": "linux"
      }
    },
...
```

```
$ cat ./blobs/sha256/afff3924849e458c5ef237db5f89539274d5e609db5db935ed3959c90f1f2d51 | jq
{
  "schemaVersion": 2,
  "config": {
    "mediaType": "application/vnd.oci.image.config.v1+json",
    "size": 7023,
    "digest": "sha256:5b0bcabd1ed22e9fb1310cf6c2dec7cdef19f0ad69efa1f392e94a4333501270"
  },
  "layers": [
    {
      "mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
      "size": 32654,
      "digest": "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f"
    },
...
```

```
$ cat ./blobs/sha256/5b0bcabd1ed22e9fb1310cf6c2dec7cdef19f0ad69efa1f392e94a4333501270 | jq
{
  "architecture": "amd64",
  "author": "Alyssa P. Hacker <alyspdev@example.com>",
  "config": {
    "Hostname": "8dfe43d80430",
    "Domainname": "",
    "User": "",
    "AttachStdin": false,
    "AttachStdout": false,
    "AttachStderr": false,
    "Tty": false,
    "OpenStdin": false,
    "StdinOnce": false,
    "Env": [
      "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
    ],
    "Cmd": null,
    "Image": "sha256:6986ae504bbf843512d680cc959484452034965db15f75ee8bdd1b107f61500b",
...
```

```
$ cat ./blobs/sha256/e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f
[tar stream]
```

[descriptors]: ./descriptor.md
