![CRI-O logo](https://github.com/cri-o/cri-o/blob/main/logo/crio-logo.svg?raw=true)

# CRI-O - OCI-based implementation of Kubernetes Container Runtime Interface

[![Stable Status](https://img.shields.io/badge/status-stable-brightgreen.svg)](#)
[![codecov](https://codecov.io/gh/cri-o/cri-o/branch/main/graph/badge.svg)](https://codecov.io/gh/cri-o/cri-o)
[![Release Notes](https://img.shields.io/badge/release-notes-blue.svg)](https://cri-o.github.io/cri-o)
[![Dependencies](https://img.shields.io/badge/report-dependencies-blue.svg)](https://cri-o.github.io/cri-o/dependencies)
[![GoDoc](https://godoc.org/github.com/cri-o/cri-o?status.svg)](https://godoc.org/github.com/cri-o/cri-o)
[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/2298/badge)](https://bestpractices.coreinfrastructure.org/projects/2298)
[![Go Report Card](https://goreportcard.com/badge/github.com/cri-o/cri-o)](https://goreportcard.com/report/github.com/cri-o/cri-o)
[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Fcri-o%2Fcri-o.svg?type=shield)](https://app.fossa.io/projects/git%2Bgithub.com%2Fcri-o%2Fcri-o?ref=badge_shield)
[![Mentioned in Awesome CRI-O](https://awesome.re/mentioned-badge.svg)](awesome.md)
[![Gitpod ready-to-code](https://img.shields.io/badge/Gitpod-ready--to--code-blue?logo=gitpod)](https://gitpod.io/#https://github.com/cri-o/cri-o)

## Compatibility matrix: CRI-O ⬄ Kubernetes

CRI-O follows the Kubernetes release cycles with respect to its minor versions
(`1.x.0`). Patch releases (`1.x.y`) for CRI-O are not in sync with those from
Kubernetes, because those are scheduled for each month, whereas CRI-O provides
them only if necessary. If a Kubernetes release goes [End of
Life](https://kubernetes.io/releases/patch-releases/),
then the corresponding CRI-O version can be considered in the same way.

This means that CRI-O also follows the Kubernetes `n-2` release version skew
policy when it comes to feature graduation, deprecation or removal. This also
applies to features which are independent from Kubernetes.

For more information visit the [Kubernetes Version Skew
Policy](https://kubernetes.io/releases/version-skew-policy/).

| Version - Branch            | Kubernetes branch/version       | Maintenance status |
| --------------------------- | ------------------------------- | ------------------ |
| CRI-O HEAD - main           | Kubernetes master branch        | ✓                  |
| CRI-O 1.24.x - release-1.24 | Kubernetes 1.24 branch, v1.24.x | =                  |
| CRI-O 1.23.x - release-1.23 | Kubernetes 1.23 branch, v1.23.x | =                  |
| CRI-O 1.22.x - release-1.22 | Kubernetes 1.22 branch, v1.22.x | =                  |
| CRI-O 1.21.x - release-1.21 | Kubernetes 1.21 branch, v1.21.x | =                  |

Key:

- `✓` Changes in the main Kubernetes repo about CRI are actively implemented in CRI-O
- `=` Maintenance is manual, only bugs will be patched.

The release notes for CRI-O are hand-crafted and can be continuously retrieved
from [our GitHub pages website](https://cri-o.github.io/cri-o).

## What is the scope of this project?

CRI-O is meant to provide an integration path between OCI conformant runtimes and the Kubelet.
Specifically, it implements the Kubelet [Container Runtime Interface (CRI)](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-node/container-runtime-interface.md) using OCI conformant runtimes.
The scope of CRI-O is tied to the scope of the CRI.

At a high level, we expect the scope of CRI-O to be restricted to the following functionalities:

- Support multiple image formats including the existing Docker image format
- Support for multiple means to download images including trust & image verification
- Container image management (managing image layers, overlay filesystems, etc)
- Container process lifecycle management
- Monitoring and logging required to satisfy the CRI
- Resource isolation as required by the CRI

## What is not in the scope of this project?

- Building, signing and pushing images to various image storages
- A CLI utility for interacting with CRI-O. Any CLIs built as part of this project are only meant for testing this project and there will be no guarantees on the backward compatibility with it.

This is an implementation of the Kubernetes Container Runtime Interface (CRI) that will allow Kubernetes to directly launch and manage Open Container Initiative (OCI) containers.

The plan is to use OCI projects and best of breed libraries for different aspects:

- Runtime: [runc](https://github.com/opencontainers/runc) (or any OCI runtime-spec implementation) and [oci runtime tools](https://github.com/opencontainers/runtime-tools)
- Images: Image management using [containers/image](https://github.com/containers/image)
- Storage: Storage and management of image layers using [containers/storage](https://github.com/containers/storage)
- Networking: Networking support through the use of [CNI](https://github.com/containernetworking/cni)

It is currently in active development in the Kubernetes community through the [design proposal](https://github.com/kubernetes/kubernetes/pull/26788). Questions and issues should be raised in the Kubernetes [sig-node Slack channel](https://kubernetes.slack.com/archives/sig-node).

## Commands

| Command                    | Description                             |
| -------------------------- | --------------------------------------- |
| [crio(8)](/docs/crio.8.md) | OCI Kubernetes Container Runtime daemon |

Note that kpod and its container management and debugging commands have moved to a separate repository, located [here](https://github.com/containers/podman).

## Configuration

| File                                                                                                     | Description                           |
| -------------------------------------------------------------------------------------------------------- | ------------------------------------- |
| [crio.conf(5)](/docs/crio.conf.5.md)                                                                     | CRI-O Configuration file              |
| [policy.json(5)](https://github.com/containers/image/blob/main/docs/containers-policy.json.5.md)         | Signature Verification Policy File(s) |
| [registries.conf(5)](https://github.com/containers/image/blob/main/docs/containers-registries.conf.5.md) | Registries Configuration file         |
| [storage.conf(5)](https://github.com/containers/storage/blob/main/docs/containers-storage.conf.5.md)     | Storage Configuration file            |

## OCI Hooks Support

[You can configure CRI-O][podman-hooks] to inject [OCI Hooks][spec-hooks] when creating containers.

## CRI-O Usage Transfer

We provide [useful information for operations and development transfer](transfer.md) as it relates to infrastructure that utilizes CRI-O.

## Communication

For async communication and long running discussions please use issues and pull requests on the GitHub repo. This will be the best place to discuss design and implementation.

For chat communication, we have a [channel on the Kubernetes slack](https://kubernetes.slack.com/archives/crio) that everyone is welcome to join and chat about development.

## Awesome CRI-O

We maintain a curated [list of links related to CRI-O](awesome.md). Did you find
something interesting on the web about the project? Awesome, feel free to open
up a PR and add it to the list.

## Getting started

### Installing CRI-O

To install `CRI-O`, you can follow our [installation guide](install.md).
Alternatively, if you'd rather build `CRI-O` from source, checkout our [setup
guide](install.md#build-and-install-cri-o-from-source).
We also provide a way in building [static binaries of `CRI-O`](install.md#static-builds) via nix.
Those binaries are available for every successfully built commit on our [Google Cloud Storage Bucket][bucket].
This means that the latest commit can be installed via our convenience script:

[bucket]: https://console.cloud.google.com/storage/browser/cri-o/artifacts

```console
> curl https://raw.githubusercontent.com/cri-o/cri-o/main/scripts/get | bash
```

The script automatically verifies the uploaded sigstore signatures as well, if
the local system has [`cosign`](https://github.com/sigstore/cosign) available in
its `$PATH`. The same applies to the [SPDX](https://spdx.org) based bill of
materials (SBOM), which gets automatically verified if the
[bom](https://sigs.k8s.io/bom) tool is in `$PATH`.

Beside `amd64` we also support the `arm64` bit architecture. This can be
selected via the script, too:

```console
> curl https://raw.githubusercontent.com/cri-o/cri-o/main/scripts/get | bash -s -- -a arm64
```

It is also possible to select a specific git SHA or tag by:

```console
> curl https://raw.githubusercontent.com/cri-o/cri-o/main/scripts/get | bash -s -- -t v1.21.0
```

The above script resolves to the download URL of the static binary bundle
tarball matching the format:

```
https://storage.googleapis.com/cri-o/artifacts/cri-o.$ARCH.$REV.tar.gz
```

where `$ARCH` can be `amd64` or `arm64` and `$REV` can be any git SHA or tag.
Please be aware that using the latest `main` SHA might cause a race, because
the CI has not finished publishing the artifacts yet or failed.

We also provide a Software Bill of Materials (SBOM) in the [SPDX
format](https://spdx.org) for each bundle. The SBOM is available at the same URL
like the bundle itself, but suffixed with `.spdx`:

```
https://storage.googleapis.com/cri-o/artifacts/cri-o.$ARCH.$REV.tar.gz.spdx
```

### Running Kubernetes with CRI-O

Before you begin, you'll need to [start CRI-O](install.md#starting-cri-o)

You can run a local version of Kubernetes with `CRI-O` using `local-up-cluster.sh`:

1. Clone the [Kubernetes repository](https://github.com/kubernetes/kubernetes)
1. From the Kubernetes project directory, run:

```console
CGROUP_DRIVER=systemd \
CONTAINER_RUNTIME=remote \
CONTAINER_RUNTIME_ENDPOINT='unix:///var/run/crio/crio.sock' \
./hack/local-up-cluster.sh
```

For more guidance in running `CRI-O`, visit our [tutorial page](tutorial.md)

[podman-hooks]: https://github.com/containers/podman/blob/v3.0.1/pkg/hooks/README.md
[spec-hooks]: https://github.com/opencontainers/runtime-spec/blob/v1.0.1/config.md#posix-platform-hooks

#### The HTTP status API

CRI-O exposes per default the [gRPC](https://grpc.io/) API to fulfill the
Container Runtime Interface (CRI) of Kubernetes. Besides this, there exists an
additional HTTP API to retrieve further runtime status information about CRI-O.
Please be aware that this API is not considered to be stable and production
use-cases should not rely on it.

On a running CRI-O instance, we can access the API via an HTTP transfer tool like
[curl](https://curl.haxx.se):

```console
$ sudo curl -v --unix-socket /var/run/crio/crio.sock http://localhost/info | jq
{
  "storage_driver": "btrfs",
  "storage_root": "/var/lib/containers/storage",
  "cgroup_driver": "systemd",
  "default_id_mappings": { ... }
}
```

The following API entry points are currently supported:

| Path              | Content-Type       | Description                                                                        |
| ----------------- | ------------------ | ---------------------------------------------------------------------------------- |
| `/info`           | `application/json` | General information about the runtime, like `storage_driver` and `storage_root`.   |
| `/containers/:id` | `application/json` | Dedicated container information, like `name`, `pid` and `image`.                   |
| `/config`         | `application/toml` | The complete TOML configuration (defaults to `/etc/crio/crio.conf`) used by CRI-O. |
| `/pause/:id`      | `application/json` | Pause a running container.                                                         |
| `/unpause/:id`    | `application/json` | Unpause a paused container.                                                        |

The tool `crio-status` can be used to access the API with a dedicated command
line tool. It supports all API endpoints via the dedicated subcommands `config`,
`info` and `containers`, for example:

```console
$ sudo go run cmd/crio-status/main.go info
cgroup driver: systemd
storage driver: btrfs
storage root: /var/lib/containers/storage
default GID mappings (format <container>:<host>:<size>):
  0:0:4294967295
default UID mappings (format <container>:<host>:<size>):
  0:0:4294967295
```

#### Metrics

Please refer to the [CRI-O Metrics guide](tutorials/metrics.md).

#### Container Runtime Interface special cases

Some aspects of the Container Runtime are worth some additional explanation.
These details are summarized in a [dedicated guide](cri.md).

#### Debugging tips

Having an issue? There are some tips and tricks for debugging located in [our debugging guide](tutorials/debugging.md)

## Adopters

An incomplete list of adopters of CRI-O in production environments can be found [here](ADOPTERS.md).
If you're a user, please help us complete it by submitting a pull-request!

## Weekly Meeting

A weekly meeting is held to discuss CRI-O development. It is open to everyone.
The details to join the meeting are on the [wiki](https://github.com/cri-o/cri-o/wiki/CRI-O-Weekly-Meeting).

## Governance

For more information on how CRI-O is goverened, take a look at the [governance file](GOVERNANCE.md)

## License Scan

[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Fcri-o%2Fcri-o.svg?type=large)](https://app.fossa.io/projects/git%2Bgithub.com%2Fcri-o%2Fcri-o?ref=badge_large)
