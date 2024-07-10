<!-- markdownlint-disable-next-line MD041 -->
![CRI-O logo](https://github.com/cri-o/cri-o/blob/main/logo/crio-logo.svg?raw=true)

# CRI-O - OCI-based implementation of Kubernetes Container Runtime Interface

<!-- markdownlint-disable-next-line MD042 -->
[![Stable Status](https://img.shields.io/badge/status-stable-brightgreen.svg)](#)
[![codecov](https://codecov.io/gh/cri-o/cri-o/branch/main/graph/badge.svg)](https://codecov.io/gh/cri-o/cri-o)
[![Packages](https://img.shields.io/badge/deb%2frpm-packages-blue.svg)](https://github.com/cri-o/packaging)
[![Release Notes](https://img.shields.io/badge/release-notes-blue.svg)](https://cri-o.github.io/cri-o)
[![Dependencies](https://img.shields.io/badge/report-dependencies-blue.svg)](https://cri-o.github.io/cri-o/dependencies)
[![GoDoc](https://godoc.org/github.com/cri-o/cri-o?status.svg)](https://godoc.org/github.com/cri-o/cri-o)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/cri-o/cri-o/badge)](https://scorecard.dev/viewer/?uri=github.com/cri-o/cri-o)
[![OpenSSF Best Practices](https://bestpractices.coreinfrastructure.org/projects/2298/badge)](https://bestpractices.coreinfrastructure.org/projects/2298)
[![Go Report Card](https://goreportcard.com/badge/github.com/cri-o/cri-o)](https://goreportcard.com/report/github.com/cri-o/cri-o)
[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Fcri-o%2Fcri-o.svg?type=shield)](https://app.fossa.io/projects/git%2Bgithub.com%2Fcri-o%2Fcri-o?ref=badge_shield)
[![Mentioned in Awesome CRI-O](https://awesome.re/mentioned-badge.svg)](awesome.md)
[![Gitpod ready-to-code](https://img.shields.io/badge/Gitpod-ready--to--code-blue?logo=gitpod)](https://gitpod.io/#https://github.com/cri-o/cri-o)
<a href="https://actuated.dev/"><img alt="Arm CI sponsored by Actuated" src="https://docs.actuated.dev/images/actuated-badge.png" height="20px"></img></a>

<!-- toc -->
- [Compatibility matrix: CRI-O ⬄ Kubernetes](#compatibility-matrix-cri-o--kubernetes)
- [What is the scope of this project?](#what-is-the-scope-of-this-project)
- [What is not in the scope of this project?](#what-is-not-in-the-scope-of-this-project)
- [Roadmap](#roadmap)
- [CI images and jobs](#ci-images-and-jobs)
- [Commands](#commands)
- [Configuration](#configuration)
- [Security](#security)
- [OCI Hooks Support](#oci-hooks-support)
- [CRI-O Usage Transfer](#cri-o-usage-transfer)
- [Communication](#communication)
- [Awesome CRI-O](#awesome-cri-o)
- [Getting started](#getting-started)
  - [Installing CRI-O](#installing-cri-o)
  - [Running Kubernetes with CRI-O](#running-kubernetes-with-cri-o)
    - [The HTTP status API](#the-http-status-api)
    - [Metrics](#metrics)
    - [Tracing](#tracing)
    - [Container Runtime Interface special cases](#container-runtime-interface-special-cases)
    - [Debugging tips](#debugging-tips)
- [Adopters](#adopters)
- [Weekly Meeting](#weekly-meeting)
- [Governance](#governance)
- [License Scan](#license-scan)
<!-- /toc -->

## Compatibility matrix: CRI-O ⬄ Kubernetes

CRI-O follows the Kubernetes release cycles with respect to its minor versions
(`1.x.y`). Patch releases (`1.x.z`) for Kubernetes are not in sync with those from
CRI-O, because they are scheduled for each month, whereas CRI-O provides
them only if necessary. If a Kubernetes release goes [End of
Life](https://kubernetes.io/releases/patch-releases/),
then the corresponding CRI-O version can be considered in the same way.

This means that CRI-O also follows the Kubernetes `n-2` release version skew
policy when it comes to feature graduation, deprecation or removal. This also
applies to features which are independent from Kubernetes. Nevertheless, feature
backports to supported release branches, which are independent from Kubernetes
or other tools like cri-tools, are still possible. This allows CRI-O to decouple
from the Kubernetes release cycle and have enough flexibility when it comes to
implement new features. Every feature to be backported will be a case by case
decision of the community while the overall compatibility matrix should not be
compromised.

For more information visit the [Kubernetes Version Skew
Policy](https://kubernetes.io/releases/version-skew-policy/).

<!-- markdownlint-disable MD013 -->
| CRI-O                           | Kubernetes                      | Maintenance status                                                    |
| ------------------------------- | ------------------------------- | --------------------------------------------------------------------- |
| `main` branch                   | `master` branch                 | Features from the main Kubernetes repository are actively implemented |
| `release-1.x` branch (`v1.x.y`) | `release-1.x` branch (`v1.x.z`) | Maintenance is manual, only bugfixes will be backported.              |
<!-- markdownlint-enable MD013 -->

The release notes for CRI-O are hand-crafted and can be continuously retrieved
from [our GitHub pages website](https://cri-o.github.io/cri-o).

## What is the scope of this project?

CRI-O is meant to provide an integration path between OCI conformant runtimes and
the Kubelet.
Specifically, it implements the Kubelet [Container Runtime Interface (CRI)](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-node/container-runtime-interface.md)
using OCI conformant runtimes.
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
- A CLI utility for interacting with CRI-O. Any CLIs built as part of this project
  are only meant for testing this project and there will be no guarantees on the
  backward compatibility with it.

CRI-O is an implementation of the Kubernetes Container Runtime Interface (CRI)
that will allow Kubernetes to directly launch and manage
Open Container Initiative (OCI) containers.

The plan is to use OCI projects and best of breed libraries for different aspects:

- Runtime: [runc](https://github.com/opencontainers/runc)
  (or any OCI runtime-spec implementation) and [oci runtime tools](https://github.com/opencontainers/runtime-tools)
- Images: Image management using [containers/image](https://github.com/containers/image)
- Storage: Storage and management of image layers using [containers/storage](https://github.com/containers/storage)
- Networking: Networking support through the use of [CNI](https://github.com/containernetworking/cni)

It is currently in active development in the Kubernetes community through the
[design proposal](https://github.com/kubernetes/kubernetes/pull/26788).
Questions and issues should be raised in the Kubernetes [sig-node Slack channel](https://kubernetes.slack.com/archives/sig-node).

## Roadmap

A roadmap that describes the direction of CRI-O can be found [here](/roadmap.md).
The project is tracking all ongoing efforts as part of the [Feature Roadmap
GitHub project](https://github.com/orgs/cri-o/projects/1).

## CI images and jobs

CRI-O's CI is split-up between GitHub actions and [OpenShift CI (Prow)](https://prow.ci.openshift.org).
Relevant virtual machine images used for the prow jobs are built periodically in
the jobs:

- [periodic-ci-cri-o-cri-o-main-periodics-setup-periodic](https://prow.ci.openshift.org/?job=periodic-ci-cri-o-cri-o-main-periodics-setup-periodic)
- [periodic-ci-cri-o-cri-o-main-periodics-setup-fedora-periodic](https://prow.ci.openshift.org/?job=periodic-ci-cri-o-cri-o-main-periodics-setup-fedora-periodic)
- [periodic-ci-cri-o-cri-o-main-periodics-evented-pleg-periodic](https://prow.ci.openshift.org/?job=periodic-ci-cri-o-cri-o-main-periodics-evented-pleg-periodic)

The jobs are maintained [from the openshift/release repository](https://github.com/openshift/release/blob/ecdeb0a/ci-operator/jobs/cri-o/cri-o/cri-o-cri-o-main-periodics.yaml)
and define workflows used for the particular jobs. The actual job definitions
can be found in the same repository under [ci-operator/jobs/cri-o/cri-o/cri-o-cri-o-main-presubmits.yaml](https://github.com/openshift/release/blob/ecdeb0a/ci-operator/jobs/cri-o/cri-o/cri-o-cri-o-main-presubmits.yaml)
for the `main` branch as well as the corresponding files for the release
branches. The base image configuration for those jobs is available in the same
repository under [ci-operator/config/cri-o/cri-o](https://github.com/openshift/release/tree/ecdeb0a/ci-operator/config/cri-o/cri-o).

## Commands

| Command                    | Description                             |
| -------------------------- | --------------------------------------- |
| [crio(8)](/docs/crio.8.md) | OCI Kubernetes Container Runtime daemon |

Examples of commandline tools to interact with CRI-O
(or other CRI compatible runtimes) are [Crictl](https://github.com/kubernetes-sigs/cri-tools/releases)
and [Podman](https://github.com/containers/podman).

## Configuration
<!-- markdownlint-disable MD013 -->
| File                                                                                                     | Description                           |
| -------------------------------------------------------------------------------------------------------- | ------------------------------------- |
| [crio.conf(5)](/docs/crio.conf.5.md)                                                                     | CRI-O Configuration file              |
| [policy.json(5)](https://github.com/containers/image/blob/main/docs/containers-policy.json.5.md)         | Signature Verification Policy File(s) |
| [registries.conf(5)](https://github.com/containers/image/blob/main/docs/containers-registries.conf.5.md) | Registries Configuration file         |
| [storage.conf(5)](https://github.com/containers/storage/blob/main/docs/containers-storage.conf.5.md)     | Storage Configuration file            |
<!-- markdownlint-enable MD013 -->

## Security

The security process for reporting vulnerabilities is described in [SECURITY.md](./SECURITY.md).

## OCI Hooks Support

[You can configure CRI-O][podman-hooks] to inject
[OCI Hooks][spec-hooks] when creating containers.

## CRI-O Usage Transfer

We provide [useful information for operations and development transfer](transfer.md)
as it relates to infrastructure that utilizes CRI-O.

## Communication

For async communication and long running discussions please use [issues](https://github.com/cri-o/cri-o/issues)
and [pull requests](https://github.com/cri-o/cri-o/pulls) on the [GitHub repo](https://github.com/cri-o/cri-o).
This will be the best place to discuss design and implementation.

For chat communication, we have a [channel on the Kubernetes slack](https://kubernetes.slack.com/archives/crio)
that everyone is welcome to join and chat about development.

## Awesome CRI-O

We maintain a curated [list of links related to CRI-O](awesome.md). Did you find
something interesting on the web about the project? Awesome, feel free to open
up a PR and add it to the list.

## Getting started

### Installing CRI-O

To install `CRI-O`, you can follow our [installation guide](install.md).
Alternatively, if you'd rather build `CRI-O` from source, checkout our [setup
guide](install.md#build-and-install-cri-o-from-source).
We also provide a way in building
[static binaries of `CRI-O`](install.md#static-builds) via nix as part of the
[cri-o/packaging repository](https://github.com/cri-o/packaging).
Those binaries are available for every successfully built commit on our
[Google Cloud Storage Bucket][bucket].
This means that the latest commit can be installed via our convenience script:

[bucket]: https://console.cloud.google.com/storage/browser/cri-o/artifacts

```console
> curl https://raw.githubusercontent.com/cri-o/packaging/main/get | bash
```

The script automatically verifies the uploaded sigstore signatures as well, if
the local system has [`cosign`](https://github.com/sigstore/cosign) available in
its `$PATH`. The same applies to the [SPDX](https://spdx.org) based bill of
materials (SBOM), which gets automatically verified if the
[bom](https://sigs.k8s.io/bom) tool is in `$PATH`.

Besides `amd64`, we also support the `arm64`, `ppc64le` and `s390x` bit
architectures. This can be selected via the script, too:

<!-- markdownlint-disable MD013 -->
```shell
curl https://raw.githubusercontent.com/cri-o/packaging/main/get | bash -s -- -a arm64
```

It is also possible to select a specific git SHA or tag by:

```shell
curl https://raw.githubusercontent.com/cri-o/packaging/main/get | bash -s -- -t v1.21.0
```
<!-- markdownlint-enable MD013 -->

The above script resolves to the download URL of the static binary bundle
tarball matching the format:

```text
https://storage.googleapis.com/cri-o/artifacts/cri-o.$ARCH.$REV.tar.gz
```

Where `$ARCH` can be `amd64`,`arm64`,`ppc64le` or `s390x` and `$REV`
can be any git SHA or tag.
Please be aware that using the latest `main` SHA might cause a race, because
the CI has not finished publishing the artifacts yet or failed.

We also provide a Software Bill of Materials (SBOM) in the [SPDX
format](https://spdx.org) for each bundle. The SBOM is available at the same URL
like the bundle itself, but suffixed with `.spdx`:

```text
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

<!-- markdownlint-disable MD013 -->
| Path              | Content-Type       | Description                                                                        |
| ----------------- | ------------------ | ---------------------------------------------------------------------------------- |
| `/info`           | `application/json` | General information about the runtime, like `storage_driver` and `storage_root`.   |
| `/containers/:id` | `application/json` | Dedicated container information, like `name`, `pid` and `image`.                   |
| `/config`         | `application/toml` | The complete TOML configuration (defaults to `/etc/crio/crio.conf`) used by CRI-O. |
| `/pause/:id`      | `application/json` | Pause a running container.                                                         |
| `/unpause/:id`    | `application/json` | Unpause a paused container.                                                        |
<!-- markdownlint-enable MD013 -->

The subcommand `crio status` can be used to access the API with a dedicated command
line tool. It supports all API endpoints via the dedicated subcommands `config`,
`info` and `containers`, for example:

```console
$ sudo crio status info
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

#### Tracing

Please refer to the [CRI-O Tracing guide](tutorials/tracing.md).

#### Container Runtime Interface special cases

Some aspects of the Container Runtime are worth some additional explanation.
These details are summarized in a [dedicated guide](cri.md).

#### Debugging tips

Having an issue? There are some tips and tricks for debugging located in
[our debugging guide](tutorials/debugging.md)

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
