![CRI-O logo](https://cdn.rawgit.com/kubernetes-sigs/cri-o/master/logo/crio-logo.svg)
# CRI-O - OCI-based implementation of Kubernetes Container Runtime Interface

[![GoDoc](https://godoc.org/github.com/cri-o/cri-o?status.svg)](https://godoc.org/github.com/cri-o/cri-o)
[![Build Status](https://img.shields.io/travis/cri-o/cri-o.svg?maxAge=2592000&style=flat-square)](https://travis-ci.org/cri-o/cri-o)
[![Go Report Card](https://goreportcard.com/badge/github.com/cri-o/cri-o?style=flat-square)](https://goreportcard.com/report/github.com/cri-o/cri-o)
[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/2298/badge)](https://bestpractices.coreinfrastructure.org/projects/2298)
[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Fcri-o%2Fcri-o.svg?type=shield)](https://app.fossa.io/projects/git%2Bgithub.com%2Fcri-o%2Fcri-o?ref=badge_shield)
[![Mentioned in Awesome CRI-O](https://awesome.re/mentioned-badge.svg)](awesome.md)

### Status: Stable

## Compatibility matrix: CRI-O <-> Kubernetes clusters

CRI-O and Kubernetes follow the same release cycle and deprecation policy. For more information visit the [Kubernetes versioning documentation](https://github.com/kubernetes/community/blob/master/contributors/design-proposals/release/versioning.md).

| Version - Branch             | Kubernetes branch/version       | Maintenance status |
|------------------------------|---------------------------------|--------------------|
| CRI-O 1.12.x  - release-1.12 | Kubernetes 1.12 branch, v1.12.x | =                  |
| CRI-O 1.13.x  - release-1.13 | Kubernetes 1.13 branch, v1.13.x | =                  |
| CRI-O 1.14.x  - release-1.14 | Kubernetes 1.14 branch, v1.14.x | =                  |
| CRI-O HEAD    - master       | Kubernetes master branch        | ✓                  |

Key:

* `✓` Changes in main Kubernetes repo about CRI are actively implemented in CRI-O
* `=` Maintenance is manual, only bugs will be patched.

## What is the scope of this project?

CRI-O is meant to provide an integration path between OCI conformant runtimes and the kubelet.
Specifically, it implements the Kubelet [Container Runtime Interface (CRI)](https://github.com/kubernetes/community/blob/master/contributors/devel/container-runtime-interface.md) using OCI conformant runtimes.
The scope of CRI-O is tied to the scope of the CRI.

At a high level, we expect the scope of CRI-O to be restricted to the following functionalities:

* Support multiple image formats including the existing Docker image format
* Support for multiple means to download images including trust & image verification
* Container image management (managing image layers, overlay filesystems, etc)
* Container process lifecycle management
* Monitoring and logging required to satisfy the CRI
* Resource isolation as required by the CRI

## What is not in scope for this project?

* Building, signing and pushing images to various image storages
* A CLI utility for interacting with CRI-O. Any CLIs built as part of this project are only meant for testing this project and there will be no guarantees on the backward compatibility with it.

This is an implementation of the Kubernetes Container Runtime Interface (CRI) that will allow Kubernetes to directly launch and manage Open Container Initiative (OCI) containers.

The plan is to use OCI projects and best of breed libraries for different aspects:
- Runtime: [runc](https://github.com/opencontainers/runc) (or any OCI runtime-spec implementation) and [oci runtime tools](https://github.com/opencontainers/runtime-tools)
- Images: Image management using [containers/image](https://github.com/containers/image)
- Storage: Storage and management of image layers using [containers/storage](https://github.com/containers/storage)
- Networking: Networking support through use of [CNI](https://github.com/containernetworking/cni)

It is currently in active development in the Kubernetes community through the [design proposal](https://github.com/kubernetes/kubernetes/pull/26788).  Questions and issues should be raised in the Kubernetes [sig-node Slack channel](https://kubernetes.slack.com/archives/sig-node).

## Commands
| Command                                              | Description                                                               | Demo|
| ---------------------------------------------------- | --------------------------------------------------------------------------|-----|
| [crio(8)](/docs/crio.8.md)                           | OCI Kubernetes Container Runtime daemon                                   ||

Note that kpod and its container management and debugging commands have moved to a separate repository, located [here](https://github.com/containers/libpod).

## Configuration
| File                                       | Description                                                                                          |
| ---------------------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| [crio.conf(5)](/docs/crio.conf.5.md)       | CRI-O Configuation file |
| [policy.json(5)](https://github.com/containers/image/blob/master/docs/containers-policy.json.5.md)     | Signature Verification Policy File(s) |
| [registries.conf(5)](https://github.com/containers/image/blob/master/docs/containers-registries.conf.5.md) | Registries Configuration file |
| [storage.conf(5)](https://github.com/containers/storage/blob/master/docs/containers-storage.conf.5.md) | Storage Configuation file |

## OCI Hooks Support

[You can configure CRI-O][libpod-hooks] to inject [OCI Hooks][spec-hooks] when creating containers.

## CRI-O Usage Transfer

[Useful information for ops and dev transfer as it relates to infrastructure that utilizes CRI-O](/transfer.md)

## Communication

For async communication and long running discussions please use issues and pull requests on the github repo. This will be the best place to discuss design and implementation.

For chat communication we have an IRC channel #CRI-O on chat.freenode.net, and a [channel on the kubernetes slack](https://kubernetes.slack.com/archives/crio) that everyone is welcome to join and chat about development.

## Awesome CRI-O

We maintain a curated [list of links related to CRI-O](awesome.md). Did you find
something interesting on the web about the project? Awesome, feel free to open
up a PR and add it to the list.

## Getting started

### Installing CRI-O
To install CRI-O, you can use your distrobutions package manager:

Fedora, CentOS, RHEL, and related distributions:
```sudo yum install crio```
openSUSE:
```sudo zypper install cri-o```

Debian, Ubuntu, and related distributions:

```bash
sudo apt-add-repository ppa:projectatomic/ppa
sudo apt-get update -qq
sudo apt-get install crio
```

Alternatively, if you'd rather build `CRI-O` from source, checkout our [setup
guide](tutorials/setup.md). We also provide a way in building [static binaries
of `CRI-O`](tutorials/setup.md#static-builds) via nix.

### Running CRI-O

You can run a local version of kubernetes with `CRI-O` using `local-up-cluster.sh`:

1. Clone the [kubernetes repository](https://github.com/kubernetes/kubernetes)
1. From the kubernetes project directory, run:
```shell
CGROUP_DRIVER=systemd \
CONTAINER_RUNTIME=remote \
CONTAINER_RUNTIME_ENDPOINT='unix:///var/run/crio/crio.sock' \
./hack/local-up-cluster.sh
```

For more guidance in running `CRI-O`, visit our [tutorial page](tutorial.md)

[libpod-hooks]: https://github.com/containers/libpod/blob/v0.6.2/pkg/hooks/README.md
[spec-hooks]: https://github.com/opencontainers/runtime-spec/blob/v1.0.1/config.md#posix-platform-hooks


## Weekly Meeting
A weekly meeting is held to discuss CRI-O development. It is open to everyone.
The details to join the meeting are on the [wiki](https://github.com/cri-o/cri-o/wiki/CRI-O-Weekly-Meeting).

## License Scan

[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Fcri-o%2Fcri-o.svg?type=large)](https://app.fossa.io/projects/git%2Bgithub.com%2Fcri-o%2Fcri-o?ref=badge_large)
