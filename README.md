![CRI-O logo](logo/crio-logo.svg)
# CRI-O - OCI-based implementation of Kubernetes Container Runtime Interface

[![Stable Status](https://img.shields.io/badge/status-stable-brightgreen.svg?style=flat-square)](#)
[![CircleCI](https://circleci.com/gh/cri-o/cri-o.svg?style=shield)](https://circleci.com/gh/cri-o/cri-o)
[![GoDoc](https://godoc.org/github.com/cri-o/cri-o?status.svg)](https://godoc.org/github.com/cri-o/cri-o)
[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/2298/badge)](https://bestpractices.coreinfrastructure.org/projects/2298)
[![Go Report Card](https://goreportcard.com/badge/github.com/cri-o/cri-o?style=flat-square)](https://goreportcard.com/report/github.com/cri-o/cri-o)
[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Fcri-o%2Fcri-o.svg?type=shield)](https://app.fossa.io/projects/git%2Bgithub.com%2Fcri-o%2Fcri-o?ref=badge_shield)
[![Mentioned in Awesome CRI-O](https://awesome.re/mentioned-badge.svg)](awesome.md)

## Compatibility matrix: CRI-O ⬄ Kubernetes

CRI-O and Kubernetes follow the same release cycle and deprecation policy. For more information visit the [Kubernetes versioning documentation](https://github.com/kubernetes/community/blob/master/contributors/design-proposals/release/versioning.md).

| Version - Branch             | Kubernetes branch/version       | Maintenance status |
|------------------------------|---------------------------------|--------------------|
| CRI-O 1.13.x  - release-1.13 | Kubernetes 1.13 branch, v1.13.x | =                  |
| CRI-O 1.14.x  - release-1.14 | Kubernetes 1.14 branch, v1.14.x | =                  |
| CRI-O 1.15.x  - release-1.15 | Kubernetes 1.15 branch, v1.15.x | =                  |
| CRI-O 1.16.x  - release-1.16 | Kubernetes 1.16 branch, v1.16.x | =                  |
| CRI-O HEAD    - master       | Kubernetes master branch        | ✓                  |

Key:

* `✓` Changes in main Kubernetes repo about CRI are actively implemented in CRI-O
* `=` Maintenance is manual, only bugs will be patched.

## What is the scope of this project?

CRI-O is meant to provide an integration path between OCI conformant runtimes and the kubelet.
Specifically, it implements the Kubelet [Container Runtime Interface (CRI)](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-node/container-runtime-interface.md) using OCI conformant runtimes.
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
| Command                                              | Description                                                               |
| ---------------------------------------------------- | --------------------------------------------------------------------------|
| [crio(8)](/docs/crio.8.md)                           | OCI Kubernetes Container Runtime daemon                                   |

Note that kpod and its container management and debugging commands have moved to a separate repository, located [here](https://github.com/containers/libpod).

## Configuration
| File                                       | Description                                                                                          |
| ---------------------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| [crio.conf(5)](/docs/crio.conf.5.md)       | CRI-O Configuration file |
| [policy.json(5)](https://github.com/containers/image/blob/master/docs/containers-policy.json.5.md)     | Signature Verification Policy File(s) |
| [registries.conf(5)](https://github.com/containers/image/blob/master/docs/containers-registries.conf.5.md) | Registries Configuration file |
| [storage.conf(5)](https://github.com/containers/storage/blob/master/docs/containers-storage.conf.5.md) | Storage Configuration file |

## OCI Hooks Support

[You can configure CRI-O][libpod-hooks] to inject [OCI Hooks][spec-hooks] when creating containers.

## CRI-O Usage Transfer

We provide [useful information for operations and development transfer](transfer.md) as it relates to infrastructure that utilizes CRI-O.

## Communication

For async communication and long running discussions please use issues and pull requests on the github repo. This will be the best place to discuss design and implementation.

For chat communication we have an IRC channel #CRI-O on chat.freenode.net, and a [channel on the Kubernetes slack](https://kubernetes.slack.com/archives/crio) that everyone is welcome to join and chat about development.

## Awesome CRI-O

We maintain a curated [list of links related to CRI-O](awesome.md). Did you find
something interesting on the web about the project? Awesome, feel free to open
up a PR and add it to the list.

## Getting started

### Installing CRI-O
To install CRI-O, you can use your distribution's package manager:

CentOS 7:
```bash
sudo curl -L -o /etc/yum.repos.d/devel:kubic:libcontainers:stable.repo https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable/CentOS_7/devel:kubic:libcontainers:stable.repo
sudo curl -L -o /etc/yum.repos.d/devel:kubic:libcontainers:stable:cri-o:[REQUIRED VERSION].repo https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable:cri-o:[REQUIRED VERSION]/CentOS_7/devel:kubic:libcontainers:stable:cri-o:[REQUIRED VERSION].repo
sudo yum -y install cri-o
```

CentOS 8:
```bash
sudo dnf -y install 'dnf-command(copr)'
sudo dnf -y copr enable rhcontainerbot/container-selinux
sudo curl -L -o /etc/yum.repos.d/devel:kubic:libcontainers:stable.repo https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable/CentOS_8/devel:kubic:libcontainers:stable.repo
sudo curl -L -o /etc/yum.repos.d/devel:kubic:libcontainers:stable:cri-o:[REQUIRED VERSION].repo https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable:cri-o:[REQUIRED VERSION]/CentOS_8/devel:kubic:libcontainers:stable:cri-o:[REQUIRED VERSION].repo
sudo dnf -y install cri-o
```

CentOS Stream:
```bash
sudo dnf -y install 'dnf-command(copr)'
sudo dnf -y copr enable rhcontainerbot/container-selinux
sudo curl -L -o /etc/yum.repos.d/devel:kubic:libcontainers:stable.repo https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable/CentOS_8_Stream/devel:kubic:libcontainers:stable.repo
sudo curl -L -o /etc/yum.repos.d/devel:kubic:libcontainers:stable:cri-o:[REQUIRED VERSION].repo https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable:cri-o:[REQUIRED VERSION]/CentOS_8_Stream/devel:kubic:libcontainers:stable:cri-o:[REQUIRED VERSION].repo
sudo dnf -y install cri-o
```

Fedora 30 and later:
```bash
sudo dnf module enable cri-o:[REQUIRED VERSION]
sudo dnf install cri-o
```

Fedora 29, RHEL, and related distributions:
```sudo yum install crio```
openSUSE:
```sudo zypper install cri-o```

Debian (10 and newer including Raspbian) and Ubuntu (18.04 and newer): Packages are available via the
[Kubic](https://build.opensuse.org/project/show/devel:kubic:libcontainers:stable)
project repositories:

```bash
# Debian Unstable/Sid
echo 'deb http://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable/Debian_Unstable/ /' > /etc/apt/sources.list.d/devel:kubic:libcontainers:stable.list
wget -nv https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable/Debian_Unstable/Release.key -O- | sudo apt-key add -

# Debian Testing
echo 'deb http://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable/Debian_Testing/ /' > /etc/apt/sources.list.d/devel:kubic:libcontainers:stable.list
wget -nv https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable/Debian_Testing/Release.key -O- | sudo apt-key add -

# Debian 10
echo 'deb http://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable/Debian_10/ /' > /etc/apt/sources.list.d/devel:kubic:libcontainers:stable.list
wget -nv https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable/Debian_10/Release.key -O- | sudo apt-key add -

# Raspbian 10
echo 'deb http://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable/Raspbian_10/ /' > /etc/apt/sources.list.d/devel:kubic:libcontainers:stable.list
wget -nv https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable/Raspbian_10/Release.key -O- | sudo apt-key add -

# Ubuntu (18.04, 19.04 and 19.10)
. /etc/os-release
sudo sh -c "echo 'deb http://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable/x${NAME}_${VERSION_ID}/ /' > /etc/apt/sources.list.d/devel:kubic:libcontainers:stable.list"
wget -nv https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable/x${NAME}_${VERSION_ID}/Release.key -O- | sudo apt-key add -

sudo apt-get update -qq
sudo apt-get install cri-o-[REQUIRED VERSION]
```

Alternatively, if you'd rather build `CRI-O` from source, checkout our [setup
guide](tutorials/setup.md). We also provide a way in building [static binaries
of `CRI-O`](tutorials/setup.md#static-builds) via nix.

### Running CRI-O

You can run a local version of Kubernetes with `CRI-O` using `local-up-cluster.sh`:

1. Clone the [Kubernetes repository](https://github.com/kubernetes/kubernetes)
1. From the Kubernetes project directory, run:
```shell
CGROUP_DRIVER=systemd \
CONTAINER_RUNTIME=remote \
CONTAINER_RUNTIME_ENDPOINT='unix:///var/run/crio/crio.sock' \
./hack/local-up-cluster.sh
```

For more guidance in running `CRI-O`, visit our [tutorial page](tutorial.md)

[libpod-hooks]: https://github.com/containers/libpod/blob/v0.6.2/pkg/hooks/README.md
[spec-hooks]: https://github.com/opencontainers/runtime-spec/blob/v1.0.1/config.md#posix-platform-hooks

#### The HTTP status API

CRI-O exposes per default the [gRPC](https://grpc.io/) API to fulfill the
Container Runtime Interface (CRI) of Kubernetes. Besides this, there exists an
additional HTTP API to retrieve further runtime status information about CRI-O.
Please be aware that this API is not considered to be stable and production
use-cases should not rely on it.

On a running CRI-O instance, we can access the API via an HTTP transfer tool like
[curl](https://curl.haxx.se):

```bash
$ sudo curl -v --unix-socket /var/run/crio/crio.sock http://localhost/info | jq
{
  "storage_driver": "btrfs",
  "storage_root": "/var/lib/containers/storage",
  "cgroup_driver": "cgroupfs",
  "default_id_mappings": { ... }
}
```

The following API entry points are currently supported:

| Path              | Content-Type       | Description                                                                        |
| ----------------- | ------------------ | ---------------------------------------------------------------------------------- |
| `/info`           | `application/json` | General information about the runtime, like `storage_driver` and `storage_root`.   |
| `/containers/:id` | `application/json` | Dedicated container information, like `name`, `pid` and `image`.                   |
| `/config`         | `application/toml` | The complete TOML configuration (defaults to `/etc/crio/crio.conf`) used by CRI-O. |

The tool `crio-status` can be used to access the API with a dedicated command
line tool. It supports all API endpoints via the dedicated subcommands `config`,
`info` and `containers`, for example:

```
$ sudo go run cmd/crio-status/main.go info
cgroup driver: cgroupfs
storage driver: btrfs
storage root: /var/lib/containers/storage
default GID mappings (format <container>:<host>:<size>):
  0:0:4294967295
default UID mappings (format <container>:<host>:<size>):
  0:0:4294967295
```

#### Metrics

Please refer to the [CRI-O Metrics guide](tutorials/metrics.md).

## Weekly Meeting
A weekly meeting is held to discuss CRI-O development. It is open to everyone.
The details to join the meeting are on the [wiki](https://github.com/cri-o/cri-o/wiki/CRI-O-Weekly-Meeting).

## License Scan

[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Fcri-o%2Fcri-o.svg?type=large)](https://app.fossa.io/projects/git%2Bgithub.com%2Fcri-o%2Fcri-o?ref=badge_large)
