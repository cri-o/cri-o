<!-- markdownlint-disable-next-line MD041 -->
![CRI-O logo](https://github.com/cri-o/cri-o/blob/main/logo/crio-logo.svg?raw=true)

# CRI-O Installation Instructions

This guide will walk you through the installation of [CRI-O](https://github.com/cri-o/cri-o),
an Open Container Initiative-based implementation of the
[Kubernetes Container Runtime Interface](https://github.com/kubernetes/community/blob/master/contributors/design-proposals/node/container-runtime-interface-v1.md).
It is assumed you are running a Linux machine.

## Table of Contents

<!-- toc -->
- [Install packaged versions of CRI-O](#install-packaged-versions-of-cri-o)
- [Build and install CRI-O from source](#build-and-install-cri-o-from-source)
  - [Runtime dependencies](#runtime-dependencies)
  - [Build and Run Dependencies](#build-and-run-dependencies)
    - [Fedora - RHEL 7 - CentOS](#fedora---rhel-7---centos)
      - [Required](#required)
    - [RHEL 8](#rhel-8)
    - [Debian - Raspbian - Ubuntu](#debian---raspbian---ubuntu)
      - [Debian up to buster - Raspbian - Ubuntu up to 18.04](#debian-up-to-buster---raspbian---ubuntu-up-to-1804)
      - [Debian bullseye or higher - Ubuntu 20.04 or higher](#debian-bullseye-or-higher---ubuntu-2004-or-higher)
  - [Get Source Code](#get-source-code)
  - [Build](#build)
    - [Install with Ansible](#install-with-ansible)
    - [Build Tags](#build-tags)
  - [Static builds](#static-builds)
  - [Download conmon](#download-conmon)
- [Setup CNI networking](#setup-cni-networking)
- [CRI-O configuration](#cri-o-configuration)
  - [Validate registries in registries.conf](#validate-registries-in-registriesconf)
  - [Optional - Modify verbosity of logs](#optional---modify-verbosity-of-logs)
  - [Optional - Modify capabilities and sysctls](#optional---modify-capabilities-and-sysctls)
- [Starting CRI-O](#starting-cri-o)
- [Using CRI-O](#using-cri-o)
- [Updating CRI-O](#updating-cri-o)
  - [openSUSE](#opensuse)
  - [Fedora 31 or later](#fedora-31-or-later)
  - [Other yum based operating systems](#other-yum-based-operating-systems)
  - [APT based operating systems](#apt-based-operating-systems)
<!-- /toc -->

## Install packaged versions of CRI-O

CRI-O follows the [Kubernetes support cycle](https://kubernetes.io/docs/setup/release/version-skew-policy/#supported-versions)
of three minor releases. CRI-O also attempts to package generically for Debian
(deb) and Red Hat (RPM) based distributions and package managers.

If there's a version or operating system that is missing, please [open an issue](https://github.com/cri-o/cri-o/issues/new).

For more information, please follow the instructions in the [CRI-O packaging repository.](https://github.com/cri-o/packaging/blob/main/README.md).

## Build and install CRI-O from source

### Runtime dependencies

- runc, crun or any other OCI compatible runtime
- iproute
- iptables

Latest version of `runc` is expected to be installed on the system. It is picked
up as the default runtime by CRI-O.

### Build and Run Dependencies

#### Fedora - RHEL 7 - CentOS

##### Required

Fedora, RHEL 7, CentOS and related distributions:

```shell
yum install -y \
  containers-common \
  git \
  glib2-devel \
  glibc-devel \
  glibc-static \
  go \
  gpgme-devel \
  libassuan-devel \
  libgpg-error-devel \
  libseccomp-devel \
  libselinux-devel \
  pkgconfig \
  make \
  runc
```

**Please note**:

- `CentOS 8` (or higher): `pkgconfig` package is replaced by `pkgconf-pkg-config`
- By default btrfs is not enabled. To add the btrfs support, install the
  following package: `btrfs-progs-devel`
- `CentOS 8`: `gpgme-devel` can be
  installed with the powertools repo.
  (`yum install -y gpgme-devel --enablerepo=powertools`)
- `CentOS 9`: `gpgme-devel` can be
  installed with the CodeReadyBuilder (crb) repo.
  (`yum install -y gpgme-devel --enablerepo=crb`)
- It is possible the distribution packaged version of runc is out of date.
- If you'd like to get the latest and greatest runc, consider using
  the one found in [devel:kubic:libcontainers:stable](https://build.opensuse.org/project/show/devel:kubic:libcontainers:stable)

#### RHEL 8

For RHEL 8 distributions (tested on RHEL 8.5).

Make sure you are subscribed to the following repositories:

- BaseOS/x86_64
- Appstream/x86_64
- CodeReady Linux Builder for x86_64

```shell
subscription-manager repos --enable=rhel-8-for-x86_64-baseos-rpms
subscription-manager repos --enable=rhel-8-for-x86_64-appstream-rpms
subscription-manager repos --enable=codeready-builder-for-rhel-8-x86_64-rpms
```

Follow [this guide to subscribe to the repositories](https://access.redhat.com/solutions/265523)
if not already subscribed.

This requires Go version 1.18 or greater. Follow [these instructions to install Go](https://go.dev/doc/install)

Install dependencies:

```shell
yum install -y \
  containers-common \
  git \
  make \
  glib2-devel \
  glibc-devel \
  glibc-static \
  runc
```

Install go-md2man:

```shell
go get github.com/cpuguy83/go-md2man
```

Install dependencies:

```shell
yum install -y \
  libassuan \
  libassuan-devel \
  libgpg-error \
  libseccomp-devel \
  libselinux \
  pkgconf-pkg-config \
  gpgme-devel \
  gcc-go
```

#### Debian - Raspbian - Ubuntu

On Debian, Raspbian and Ubuntu distributions, [enable the Kubic project
repositories](#apt-based-operating-systems) (for `containers-common`
and `cri-o-runc` packages) and install the following packages:

##### Debian up to buster - Raspbian - Ubuntu up to 18.04

```shell
apt update -qq && \
# For Debian 10(buster) or below: use "apt install -t buster-backports"
apt install -y  \
  btrfs-tools \
  containers-common \
  git \
  libassuan-dev \
  libglib2.0-dev \
  libc6-dev \
  libgpgme11-dev \
  libgpg-error-dev \
  libseccomp-dev \
  libsystemd-dev \
  libbtrfs-dev \
  libselinux1-dev \
  pkg-config \
  go-md2man \
  cri-o-runc \
  libudev-dev \
  software-properties-common \
  gcc \
  make
```

##### Debian bullseye or higher - Ubuntu 20.04 or higher

```shell
apt-get update -qq && apt-get install -y \
  libbtrfs-dev \
  containers-common \
  git \
  libassuan-dev \
  libglib2.0-dev \
  libc6-dev \
  libgpgme-dev \
  libgpg-error-dev \
  libseccomp-dev \
  libsystemd-dev \
  libselinux1-dev \
  pkg-config \
  go-md2man \
  cri-o-runc \
  libudev-dev \
  software-properties-common \
  gcc \
  make
```

**Caveats and Notes:**

If using an older release or a long-term support release, be careful to
double-check that the version of `runc` is new enough (running `runc --version`
should produce `spec: 1.0.0` or greater), or else build your own.

Be careful to double-check that the version of golang is new enough, version
1.12.x or higher is required. If needed, newer golang versions are available at
[the official download website](https://golang.org/dl).

### Get Source Code

Clone the source code using:

```shell
git clone https://github.com/cri-o/cri-o # or your fork
cd cri-o
```

Make sure your `CRI-O` and `kubernetes` versions are of matching major versions.
For instance, if you want to be compatible with the latest kubernetes release,
you'll need to use the latest tagged release of `CRI-O` on branch `release-1.18`.

### Build

To install with default buildtags using seccomp, use:

```shell
make
sudo make install
```

Otherwise, if you do not want to build `CRI-O` with seccomp support you can add
`BUILDTAGS=""` when running make.

```shell
make BUILDTAGS=""
sudo make install
```

#### Install with Ansible

An [Ansible Role](https://github.com/alvistack/ansible-role-cri_o) is also
available to automate the above steps:

``` bash
sudo su -
mkdir -p ~/.ansible/roles
cd ~/.ansible/roles
git clone https://github.com/alvistack/ansible-role-cri_o.git cri_o
cd ~/.ansible/roles/cri_o
pip3 install --upgrade --ignore-installed --requirement requirements.txt
molecule converge
molecule verify
```

#### Build Tags

`CRI-O` supports optional build tags for compiling support of various features.
To add build tags to the make option the `BUILDTAGS` variable must be set.

```shell
make BUILDTAGS='seccomp apparmor'
```

| Build Tag | Feature                            | Dependency |
| --------- | ---------------------------------- | ---------- |
| seccomp   | syscall filtering                  | libseccomp |
| selinux   | selinux process and mount labeling | libselinux |
| apparmor  | apparmor profile support           |            |

`CRI-O` manages images with [containers/image](https://github.com/containers/image),
which uses the following buildtags.

<!-- markdownlint-disable MD013 -->
| Build Tag                    | Feature                                     | Dependency |
| ---------------------------- | ------------------------------------------- | ---------- |
| containers_image_openpgp     | use native golang pgp instead of cgo        |            |
| containers_image_ostree_stub | disable use of ostree as an image transport |            |

`CRI-O` also uses [containers/storage](https://github.com/containers/storage) for managing container storage.

| Build Tag                        | Feature                                         | Dependency   |
| -------------------------------- | ----------------------------------------------- | ------------ |
| exclude_graphdriver_btrfs        | exclude btrfs as a storage option               |              |
| btrfs_noversion                  | for building btrfs version < 3.16.1             | btrfs        |
| exclude_graphdriver_overlay      | exclude overlay as a storage option             |              |
| ostree                           | build storage using ostree                      | ostree       |
<!-- markdownlint-enable MD013 -->

### Static builds

It is possible to build a statically linked binary of CRI-O by using the
officially provided [nix](https://nixos.org/nix) package and the derivation of
it [within this repository](../nix). The builds are completely reproducible and
will create a `x86_64`/`amd64` or `aarch64`/`arm64`, `ppc64le` or `s390x`
stripped ELF binary for [glibc](https://www.gnu.org/software/libc) or [musl
libc (for `s390x`)](https://www.musl-libc.org/). These binaries are integration tested
(for `amd64` and `arm64`) as well and support the following features:

- apparmor
- btrfs
- gpgme
- seccomp
- selinux

To build the binaries locally either [install the nix package
manager](https://nixos.org/nix/download.html) or use the `make build-static`
target which relies on the nixos/nix container image.

The overall build process can take a tremendous amount of CPU time depending on
the hardware. The resulting binaries should now be available within:

- `bin/static/crio`

To build the binaries without any prepared container and via the already
installed nix package manager, simply run the following command from the root
directory of this repository:

```shell
nix build -f nix
```

The resulting binaries should be now available in `result/bin`. To build the arm
variant of the binaries, just run:

```shell
nix build -f nix/default-arm64.nix
```

Similarly, the ppc64le variant of binaries can be built using:

```shell
nix build -f nix/default-ppc64le.nix
```

In the same way, the s390x variant of binaries can be built using:

```shell
nix build -f nix/default-s390x.nix
```

### Download conmon

[conmon](https://github.com/containers/conmon) is a per-container daemon that
`CRI-O` uses to monitor container logs and exit information.
`conmon` needs to be downloaded with `CRI-O`.

running:

```shell
git clone https://github.com/containers/conmon
cd conmon
make
sudo make install
```

will download conmon to your $PATH.

## Setup CNI networking

A proper description of setting up CNI networking is given in the
[`contrib/cni` README](/contrib/cni/README.md). But the gist is that you need to
have some basic network configurations enabled and CNI plugins installed on
your system.

## CRI-O configuration

If you are installing for the first time, generate and install
configuration files with:

```shell
sudo make install.config
```

### Validate registries in registries.conf

Edit `/etc/containers/registries.conf` and verify that the registries option has
valid values in it.  For example:

<!-- markdownlint-disable MD013 -->
```conf
[registries.search]
registries = ['registry.access.redhat.com', 'registry.fedoraproject.org', 'quay.io', 'docker.io']

[registries.insecure]
registries = []

[registries.block]
registries = []
```
<!-- markdownlint-enable MD013 -->

For more information about this file see [registries.conf(5)](https://github.com/containers/image/blob/main/docs/containers-registries.conf.5.md).

### Optional - Modify verbosity of logs

Users can modify the `log_level` by specifying an overwrite like
`/etc/crio/crio.conf.d/01-log-level.conf` to change the verbosity of
the logs. Options are fatal, panic, error, warn, info (default), debug and
trace.

```conf
[crio.runtime]
log_level = "info"
```

### Optional - Modify capabilities and sysctls

By default, `CRI-O` uses the following capabilities:

```conf
default_capabilities = [
  "CHOWN",
  "DAC_OVERRIDE",
  "FSETID",
  "FOWNER",
  "SETGID",
  "SETUID",
  "SETPCAP",
  "NET_BIND_SERVICE",
  "KILL",
]
```

and no sysctls

```conf
default_sysctls = [
]
```

Users can change either default by adding overwrites to `/etc/crio/crio.conf.d`.

## Starting CRI-O

Running make install will download CRI-O into the folder

```shell
/usr/local/bin/crio
```

You can run it manually there, or you can set up a systemd unit file with:

```shell
sudo make install.systemd
```

And let systemd take care of running CRI-O:

``` bash
sudo systemctl daemon-reload
sudo systemctl enable crio
sudo systemctl start crio
```

## Using CRI-O

- Follow this [tutorial](tutorials/crictl.md) to quickly get started running
  simple pods and containers.
- To run a full cluster, see [the instructions](tutorials/kubernetes.md).
- To run with kubeadm, see [kubeadm instructions](tutorials/kubeadm.md).

## Updating CRI-O

<!-- markdownlint-disable MD024 -->
### openSUSE

```shell
sudo zypper update
sudo zypper update cri-o
```

### Fedora 31 or later

```shell
sudo dnf update
sudo dnf update cri-o
```

### Other yum based operating systems

```shell
sudo yum update
sudo yum update cri-o
```

### APT based operating systems

```shell
sudo apt upgrade cri-o
```
