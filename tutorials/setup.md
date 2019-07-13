# Build and install CRI-O from source

This guide will walk you through the installation of [CRI-O](https://github.com/cri-o/cri-o), an Open Container Initiative-based implementation of [Kubernetes Container Runtime Interface](https://github.com/kubernetes/community/blob/master/contributors/design-proposals/node/container-runtime-interface-v1.md).

It is assumed you are running a Linux machine.

## Runtime dependencies

- runc, Clear Containers runtime, or any other OCI compatible runtime
- socat
- iproute
- iptables

Latest version of `runc` is expected to be installed on the system. It is picked up as the default runtime by CRI-O.

## Build and Run Dependencies

**Required**

Fedora, RHEL<=7, CentOS and related distributions:

```bash
yum install -y \
  btrfs-progs-devel \
  containers-common \
  device-mapper-devel \
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
  runc
```

RHEL 8 distributions:
Make sure you are subscribed to the following repositories: \
BaseOS/x86_64 \
Appstream/x86_64

Follow the guide below to subscribe to the repositories if not already subscribed:\
https://access.redhat.com/solutions/265523

This requires go version 1.12 or greater.

```bash
yum install -y \
  containers-common \
  device-mapper-devel \
  git \
  glib2-devel \
  glibc-devel \
  glibc-static \
  runc \
```

Here is a link on how to install a source rpm on RHEL: \
https://www.itechlounge.net/2012/12/linux-how-to-install-source-rpm-on-rhelcentos/

Dependency: gpgme-devel \
Link: http://download.eng.bos.redhat.com/brewroot/packages/gpgme/1.10.0/6.el8/x86_64/

Dependency: go-md2man \
Command: go get github.com/cpuguy83/go-md2man

The following dependencies:
```bash
  libassuan \
  libgpg-error \
  libseccomp \
  libselinux \
  pkgconfig \
```
are found on the following page: \
Link: https://downloads.redhat.com/redhat/rhel/rhel-8-beta/baseos/source/Packages/

On Ubuntu distributions, there is a dedicated PPA provided by
[Project Atomic](https://www.projectatomic.io/):

```bash
# Add containers-common and cri-o-runc
apt-add-repository ppa:projectatomic/ppa

apt-get update -qq && apt-get install -y \
  btrfs-tools \
  containers-common \
  git \
  golang-go \
  libassuan-dev \
  libdevmapper-dev \
  libglib2.0-dev \
  libc6-dev \
  libgpgme11-dev \
  libgpg-error-dev \
  libseccomp-dev \
  libsystemd-dev \
  libselinux1-dev \
  pkg-config \
  go-md2man \
  cri-o-runc
```

**Caveats and Notes:**

If using an older release or a long-term support release, be careful to double-check that the version of `runc` is new enough (running `runc --version` should produce `spec: 1.0.0`), or else build your own.

Be careful to double-check that the version of golang is new enough, version 1.10.x or higher is required.  If needed, golang kits are available at https://golang.org/dl/

## Get Source Code

Clone the source code using:

```bash
git clone https://github.com/cri-o/cri-o # or your fork
cd cri-o
```

Make sure your `CRI-O` and `kubernetes` versions are of matching major versions. For instance, if you want to be compatible with the latest kubernetes release, you'll need to use the latest tagged release of `CRI-O` on branch release-1.15

## Build

To install with default buildtags using seccomp, use:

```bash
make
sudo make install
```

Otherwise, if you do not want to build `CRI-O` with seccomp support you can add `BUILDTAGS=""` when running make.

```bash
make BUILDTAGS=""
sudo make install
```

### Build Tags

`CRI-O` supports optional build tags for compiling support of various features.
To add build tags to the make option the `BUILDTAGS` variable must be set.

```bash
make BUILDTAGS='seccomp apparmor'
```

| Build Tag                        | Feature                                         | Dependency   |
|----------------------------------|-------------------------------------------------|--------------|
| seccomp                          | syscall filtering                               | libseccomp   |
| selinux                          | selinux process and mount labeling              | libselinux   |
| apparmor                         | apparmor profile support                        | <none>       |

`CRI-O` manages images with [containers/image](https://github.com/containers/image), which uses the following buildtags.

| Build Tag                        | Feature                                         | Dependency   |
|----------------------------------|-------------------------------------------------|--------------|
| containers_image_openpgp         | use native golang pgp instead of cgo            | <none>       |
| containers_image_ostree_stub     | disable use of ostree as an image transport     | <none>       |

`CRI-O` also uses [containers/storage](https://github.com/containers/storage) for managing container storage.

| Build Tag                        | Feature                                         | Dependency   |
|----------------------------------|-------------------------------------------------|--------------|
| exclude_graphdriver_btrfs        | exclude btrfs as a storage option               | <none>       |
| btrfs_noversion                  | for building btrfs version < 3.16.1             | btrfs        |
| exclude_graphdriver_devicemapper | exclude devicemapper as a storage option        | <none>       |
| libdm_no_deferred_remove         | don't compile deferred remove with devicemapper | devicemapper |
| exclude_graphdriver_overlay      | exclude overlay as a storage option             | <none>       |
| ostree                           | build storage using ostree                      | ostree       |

## Static builds

It is possible to build a statically linked binary of CRI-O by using the
officially provided [nix](https://nixos.org/nix) package and the derivation of
it [within this repository](../nix). The builds are completely reproducible and
will create a `x86_64`/`amd64` stripped ELF binary for
[glibc](https://www.gnu.org/software/libc) and [musl
libc](https://www.musl-libc.org).  These binaries are integration tested as well
and support the following features:

- apparmor
- btrfs
- device mapper
- gpgme
- seccomp
- selinux

To build the binaries locally either [install the nix package
manager](https://nixos.org/nix/download.html) or setup a new container image
from the root directory of this repository by executing:

```
make nix-image
```

Please note that you can specify the container runtime and image name by
specifying:

```
make nix-image \
    CONTAINER_RUNTIME=podman \
    NIX_IMAGE=crionix
```

The overall build process can take a tremendous amount of CPU time depending on
the hardware. After the image has been successfully built, it should be possible
to build the binaries:

```
make build-static
```

There exist an already pre-built container image used for the internal CI. This
means that invoking `make build-static` should work even without building the
image before.

Note that the container runtime and nix image can be specified here, too. The
resulting binaries should now be available within:

- `bin/crio-x86_64-static-glibc`
- `bin/crio-x86_64-static-musl`

- `bin/pause-x86_64-static-glibc`
- `bin/pause-x86_64-static-musl`

To build the binaries without any prepared container and via the already
installed nix package manager, simply run the following command from the root
directory of this repository:

```
nix-build nix
```

The resulting binary should be now available in `result-bin/bin` and
`result-2-bin/bin`.

### Creating a release archive

A release bundle consists of all static binaries, the man pages and
configuration files like `crio.conf`. The `release-bundle` target can be used to
build a new release archive within the current repository:

```
make release-bundle
...
Created ./bundle/crio-v1.15.0.tar.gz
```

## Setup CNI networking

A proper description of setting up CNI networking is given in the
[`contrib/cni` README](/contrib/cni/README.md). But the gist is that you need to
have some basic network configurations enabled and CNI plugins installed on
your system.

## CRI-O configuration

If you are installing for the first time, generate and install configuration files with:

```
sudo make install.config
```

### Validate registries in registries.conf

Edit `/etc/containers/registries.conf` and verify that the registries option has valid values in it.  For example:

```
[registries.search]
registries = ['registry.access.redhat.com', 'registry.fedoraproject.org', 'quay.io', 'docker.io']

[registries.insecure]
registries = []

[registries.block]
registries = []
```

For more information about this file see [registries.conf(5)](https://github.com/containers/image/blob/master/docs/containers-registries.conf.5.md).

### Recommended - Use systemd cgroups.

By default, CRI-O uses cgroupfs as a cgroup driver. However, we recommend using systemd as a cgroup driver. You can change your cgroup driver in crio.conf:

```
cgroup_driver = "systemd"
```

### Optional - Modify verbosity of logs in /etc/crio/crio.conf

Users can modify the `log_level` field in `/etc/crio/crio.conf` to change the verbosity of
the logs.
Options are fatal, panic, error (default), warn, info, and debug.

```
log_level = "info"
```

### Optional - Modify capabilities and sysctls
By default, `CRI-O` uses the following capabilities:

```
default_capabilities = [
	"CHOWN",
	"DAC_OVERRIDE",
	"FSETID",
	"FOWNER",
	"NET_RAW",
	"SETGID",
	"SETUID",
	"SETPCAP",
	"NET_BIND_SERVICE",
	"SYS_CHROOT",
	"KILL",
]
```
and no sysctls
```
default_sysctls = [
]
```

Users can change either default by editing `/etc/crio/crio.conf`.

## Starting CRI-O

Running make install will download CRI-O into the folder
```bash
/usr/local/bin/crio
```

You can run it manually there, or you can set up a systemd unit file with:

```
sudo make install.systemd
```

And let systemd take care of running CRI-O:

``` bash
sudo systemctl daemon-reload
sudo systemctl enable crio
sudo systemctl start crio
```

## Using CRI-O

- Follow this [tutorial](crictl.md) to quickly get started running simple pods and containers.
- To run a full cluster, see [the instructions](kubernetes.md).
- To run with kubeadm, see [kubeadm instructions](kubeadm.md).
