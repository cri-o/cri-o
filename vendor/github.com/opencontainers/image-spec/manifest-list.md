# OCI Image Manifest List Specification

The manifest list is a higher-level manifest which points to specific [image manifests](manifest.md) for one or more platforms.
While the use of a manifest list is OPTIONAL for image providers, image consumers SHOULD be prepared to process them.

This section defines the `application/vnd.oci.image.manifest.list.v1+json` [media type](media-types.md).
For the media type(s) that this document is compatible with, see the [matrix][matrix].

## *Manifest List* Property Descriptions

- **`schemaVersion`** *int*

  This REQUIRED property specifies the image manifest schema version.
  For this version of the specification, this MUST be `2` to ensure backward compatibility with older versions of Docker. The value of this field will not change. This field MAY be removed in a future version of the specification.

- **`mediaType`** *string*

  This property is *reserved* for use, to [maintain compatibility][matrix].
  When used, this field contains the media type of this document, which differs from the [descriptor](descriptor.md#properties) use of `mediaType`.

- **`manifests`** *array of objects*

  This REQUIRED property contains a list of manifests for specific platforms.
  While the property MUST be present, the size of the array MAY be zero.

  Each object in `manifests` is a [descriptor](descriptor.md) with the following additional properties and restrictions:

  - **`mediaType`** *string*

    This [descriptor property](descriptor.md#properties) has additional restrictions for `manifests`.
    Implementations MUST support at least the following media types:

    - [`application/vnd.oci.image.manifest.v1+json`](manifest.md)

    Manifest lists concerned with portability SHOULD use one of the above media types.

  - **`platform`** *object*

    This REQUIRED property describes the platform which the image in the manifest runs on.

    - **`architecture`** *string*

        This REQUIRED property specifies the CPU architecture.
        Manifest lists SHOULD use, and implementations SHOULD understand, values [supported by runtime-spec's `platform.arch`][runtime-platform2].

    - **`os`** *string*

        This REQUIRED property specifies the operating system.
        Manifest lists SHOULD use, and implementations SHOULD understand, values [supported by runtime-spec's `platform.os`][runtime-platform2].

    - **`os.version`** *string*

        This OPTIONAL property specifies the operating system version, for example `10.0.10586`.

    - **`os.features`** *array of strings*

        This OPTIONAL property specifies an array of strings, each specifying a mandatory OS feature (for example on Windows `win32k`).

    - **`variant`** *string*

        This OPTIONAL property specifies the variant of the CPU, for example `armv6l` to specify a particular CPU variant of the ARM CPU.

    - **`features`** *array of strings*

        This OPTIONAL property specifies an array of strings, each specifying a mandatory CPU feature (for example `sse4` or `aes`).

- **`annotations`** *string-string map*

    This OPTIONAL property contains arbitrary metadata for the manifest list.
    This OPTIONAL property MUST use the [annotation rules](annotations.md#rules).

    See [Pre-Defined Annotation Keys](annotations.md#pre-defined-annotation-keys).

## Example Manifest List

*Example showing a simple manifest list pointing to image manifests for two platforms:*
```json,title=Manifest%20List&mediatype=application/vnd.oci.image.manifest.list.v1%2Bjson
{
  "schemaVersion": 2,
  "manifests": [
    {
      "mediaType": "application/vnd.oci.image.manifest.v1+json",
      "size": 7143,
      "digest": "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f",
      "platform": {
        "architecture": "ppc64le",
        "os": "linux"
      }
    },
    {
      "mediaType": "application/vnd.oci.image.manifest.v1+json",
      "size": 7682,
      "digest": "sha256:5b0bcabd1ed22e9fb1310cf6c2dec7cdef19f0ad69efa1f392e94a4333501270",
      "platform": {
        "architecture": "amd64",
        "os": "linux",
        "os.features": [
          "sse4"
        ]
      }
    }
  ],
  "annotations": {
    "com.example.key1": "value1",
    "com.example.key2": "value2"
  }
}
```

[runtime-platform2]: https://github.com/opencontainers/runtime-spec/blob/v1.0.0-rc2/config.md#platform
[matrix]: media-types.md#compatibility-matrix
