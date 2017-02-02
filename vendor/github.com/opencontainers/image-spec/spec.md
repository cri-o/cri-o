# Open Container Initiative Image Format Specification

This specification defines an OCI Image, consisting of a [manifest](manifest.md), a [manifest list](manifest-list.md) (optional), a set of [filesystem layers](layer.md), and a [configuration](config.md).

The goal of this specification is to enable the creation of interoperable tools for building, transporting, and preparing a container image to run.

## Table of Contents

- [Introduction](spec.md)
- [Notational Conventions](#notational-conventions)
- [Overview](#overview)
    - [Understanding the Specification](#understanding-the-specification)
    - [Media Types](media-types.md)
- [Content Descriptors](descriptor.md)
- [Image Layout](image-layout.md)
- [Image Manifest](manifest.md)
- [Image Manifest List](manifest-list.md)
- [Filesystem Layers](layer.md)
- [Image Configuration](config.md)
- [Annotations](annotations.md)
- [Considerations](considerations.md)
    - [Extensibility](considerations.md#extensibility)
    - [Canonicalization](considerations.md#canonicalization)

# Notational Conventions

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD", "SHOULD NOT", "RECOMMENDED", "MAY", and "OPTIONAL" are to be interpreted as described in [RFC 2119](http://tools.ietf.org/html/rfc2119) (Bradner, S., "Key words for use in RFCs to Indicate Requirement Levels", BCP 14, RFC 2119, March 1997).

The key words "unspecified", "undefined", and "implementation-defined" are to be interpreted as described in the [rationale for the C99 standard][c99-unspecified].

An implementation is not compliant if it fails to satisfy one or more of the MUST, REQUIRED, or SHALL requirements for the protocols it implements.
An implementation is compliant if it satisfies all the MUST, REQUIRED, and SHALL requirements for the protocols it implements.

# Overview

At a high level the image manifest contains metadata about the contents and dependencies of the image including the content-addressable identity of one or more [filesystem layer changeset](layer.md) archives that will be unpacked to make up the final runnable filesystem.
The image configuration includes information such as application arguments, environments, etc.
The manifest list is a higher-level manifest which points to one or more manifests.
Typically, these manifests may provide different implementations of the image, possibly varying by platform or other attributes.

![](img/build-diagram.png)

Once built the OCI Image can then be discovered by name, downloaded, verified by hash, trusted through a signature, and unpacked into an [OCI Runtime Bundle](https://github.com/opencontainers/runtime-spec/blob/master/bundle.md).

![](img/run-diagram.png)

## Understanding the Specification

The [OCI Image Media Types](media-types.md) document is a starting point to understanding the overall structure of the specification.

The high-level components of the spec include:

* An archival format for container images, consisting of an [image manifest](manifest.md), a [manifest list](manifest-list.md) (optional), an [image layout](image-layout.md), a set of [filesystem layers](layer.md), and [image configuration](config.md) (base OCI layer)
* A [process of referencing container images by a cryptographic hash of their content](descriptor.md) (base OCI layer)
* A format for [storing CAS blobs and references to them](image-layout.md) (optional OCI layer)
* Signatures that are based on signing image content address (optional OCI layer)
* Naming that is federated based on DNS and can be delegated (optional OCI layer)

[c99-unspecified]: http://www.open-std.org/jtc1/sc22/wg14/www/C99RationaleV5.10.pdf#page=18
