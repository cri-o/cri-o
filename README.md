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

`glib2-devel` package on Fedora or ` libglib2.0-dev` on Ubuntu or equivalent is required.


```
$ GOPATH=/path/to/gopath
$ mkdir $GOPATH
$ go get -d github.com/kubernetes-incubator/cri-o
$ cd $GOPATH/src/github.com/kubernetes-incubator/cri-o
$ make install.tools
$ make binaries
$ make docs
$ sudo make install
```


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
$ ocic pod create --config test/testdata/sandbox_config.json
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

### Current Roadmap

1. Basic pod/container lifecycle, basic image pull (already works)
1. Support for tty handling and state management
1. Basic integration with kubelet once client side changes are ready
1. Support for log management, networking integration using CNI, pluggable image/storage management
1. Support for exec/attach
1. Target fully automated kubernetes testing without failures
