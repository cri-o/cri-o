# cri-o - OCI-based implementation of Kubernetes Container Runtime Interface

[![Build Status](https://img.shields.io/travis/kubernetes-incubator/cri-o.svg?maxAge=2592000&style=flat-square)](https://travis-ci.org/kubernetes-incubator/cri-o)
[![Go Report Card](https://goreportcard.com/badge/github.com/kubernetes-incubator/cri-o?style=flat-square)](https://goreportcard.com/report/github.com/kubernetes-incubator/cri-o)

### Status: pre-alpha

## What is the scope of this project?

cri-o is meant to provide an integration path between OCI conformant runtimes and the kubelet.
Specifically, it implements the Kubelet Container Runtime Interface (CRI) using OCI conformant runtimes.
The scope of cri-o is tied to the scope of the CRI.

At a high level, we expect the scope of cri-o to be restricted to the following functionalities:

* Support multiple image formats including the existing Docker image format
* Support for multiple means to download images including trust & image verification
* Container image management (managing image layers, overlay filesystems, etc)
* Container process lifecycle management
* Monitoring and logging required to satisfy the CRI
* Resource isolation as required by the CRI

## What is not in scope for this project?

* Building, signing and pushing images to various image storages
* A CLI utility for interacting with cri-o. Any CLIs built as part of this project are only meant for testing this project and there will be no guarantees on the backwards compatibility with it.

This is an implementation of the Kubernetes Container Runtime Interface (CRI) that will allow Kubernetes to directly launch and manage Open Container Initiative (OCI) containers.

The plan is to use OCI projects and best of breed libraries for different aspects:
- Runtime: [runc](https://github.com/opencontainers/runc) (or any OCI runtime-spec implementation) and [oci runtime tools](https://github.com/opencontainers/runtime-tools)
- Images: Image management using [containers/image](https://github.com/containers/image)
- Storage: Storage and management of image layers using [containers/storage](https://github.com/containers/storage)
- Networking: Networking support through use of [CNI](https://github.com/containernetworking/cni)

It is currently in active development in the Kubernetes community through the [design proposal](https://github.com/kubernetes/kubernetes/pull/26788).  Questions and issues should be raised in the Kubernetes [sig-node Slack channel](https://kubernetes.slack.com/archives/sig-node).

## Getting started

### Prerequisites
`runc` version 1.0.0.rc1 or greater is expected to be installed on the system. It is picked up as the default runtime by ocid.

### Build

`btrfs-progs-devel`, `device-mapper-devel`, `glib2-devel`, `glibc-devel`, `gpgme-devel`, `libassuan-devel`, `libgpg-error-devel`, and `pkg-config` packages on CentOS/Fedora or `btrfs-tools`, `libassuan-dev`, `libc6-dev`, `libdevmapper-dev`, `libglib2.0-dev`, `libgpg-error-dev`, `libgpgme11-dev`, and `pkg-config` on Ubuntu or equivalent is required.
In order to enable seccomp support you will need to install development files for `libseccomp` on your platform.
> e.g. `libseccomp-devel` for CentOS/Fedora, or `libseccomp-dev` for Ubuntu
In order to enable apparmor support you will need to install development files for `libapparmor` on your platform.
> e.g. `libapparmor-dev` for Ubuntu

```bash
$ GOPATH=/path/to/gopath
$ mkdir $GOPATH
$ go get -d github.com/kubernetes-incubator/cri-o
$ cd $GOPATH/src/github.com/kubernetes-incubator/cri-o
$ make install.tools
$ make
$ sudo make install
```
Otherwise, if you do not want to build `cri-o` with seccomp support you can add `BUILDTAGS=""` when running make.

```bash
# create a 'github.com/kubernetes-incubator' in your $GOPATH/src
cd github.com/kubernetes-incubator
git clone https://github.com/kubernetes-incubator/cri-o
cd cri-o

make BUILDTAGS=""
sudo make install
```

#### Build Tags

`cri-o` supports optional build tags for compiling support of various features.
To add build tags to the make option the `BUILDTAGS` variable must be set.

```bash
make BUILDTAGS='seccomp apparmor'
```

| Build Tag | Feature                            | Dependency  |
|-----------|------------------------------------|-------------|
| seccomp   | syscall filtering                  | libseccomp  |
| selinux   | selinux process and mount labeling | <none>      |
| apparmor  | apparmor profile support           | libapparmor |

### Running pods and containers

#### Start the server
```
# ocid --debug
```
If the default `--runtime` value does not point to your runtime:
```
# ocid --runtime $(which runc)
```

#### Create a pod
```
$ ocic pod run --config test/testdata/sandbox_config.json
```

#### Get pod status
```
# ocic pod status --id <pod_id>
```

#### Run a container inside a pod
```
# ocic ctr create --pod <pod_id> --config test/testdata/container_redis.json
```

#### Start a container
```
# ocic ctr start --id <ctr_id>
```

#### Get container status
```
# ocic ctr status --id <ctr_id>
```

#### Stop a container
```
# ocic ctr stop --id <ctr_id>
```

#### Remove a container
```
# ocic ctr remove --id <ctr_id>
```

#### Stop a pod
```
# ocic pod stop --id <pod_id>
```

#### Remove a pod
```
# ocic pod remove --id <pod_id>
```

#### List pods
```
# ocic pod list
```

#### List containers
```
# ocic ctr list
```

### Setup CNI networking

Follow the steps below in order to setup networking in your pods using the CNI
bridge plugin. Nothing else is required after this since `CRI-O` automatically
setup networking if it finds any CNI plugin.

```sh
$ go get -d github.com/containernetworking/cni
$ cd $GOPATH/src/github.com/containernetworking/cni
$ sudo mkdir -p /etc/cni/net.d
$ sudo sh -c 'cat >/etc/cni/net.d/10-mynet.conf <<-EOF
{
    "cniVersion": "0.2.0",
    "name": "mynet",
    "type": "bridge",
    "bridge": "cni0",
    "isGateway": true,
    "ipMasq": true,
    "ipam": {
        "type": "host-local",
        "subnet": "10.88.0.0/16",
        "routes": [
            { "dst": "0.0.0.0/0"  }
        ]
    }
}
EOF'
$ sudo sh -c 'cat >/etc/cni/net.d/99-loopback.conf <<-EOF
{
    "cniVersion": "0.2.0",
    "type": "loopback"
}
EOF'
$ ./build
$ sudo mkdir -p /opt/cni/bin
$ sudo cp bin/* /opt/cni/bin/
```

### Current Roadmap

1. Basic pod/container lifecycle, basic image pull (already works)
1. Support for tty handling and state management
1. Basic integration with kubelet once client side changes are ready
1. Support for log management, networking integration using CNI, pluggable image/storage management
1. Support for exec/attach
1. Target fully automated kubernetes testing without failures
