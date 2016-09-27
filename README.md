OCID - OCI-based implementation of Kubernetes Container Runtime Interface [![Build Status](https://travis-ci.org/kubernetes-incubator/cri-o.svg?branch=master)](https://travis-ci.org/kubernetes-incubator/cri-o)
=

### Status: pre-alpha

# What is the scope of this project?

OCID is meant to provide an integration path between OCI conformant runtimes and the kubelet.
Specifically, it implements the Kubelet Container Runtime Interface (CRI) using OCI conformant runtimes.
The scope of OCID is tied to the scope of the CRI.

At a high level, we expect the scope of OCID to be restricted to the following functionalities:

* Support multiple image formats including the existing Docker image format 
* Support for multiple means to download images including trust & image verification
* Container image management (managing image layers, overlay filesystems, etc)
* Container process lifecycle management
* Monitoring and logging required to satisfy the CRI
* Resource isolation as required by the CRI

# What is not in scope for this project?

* Building, signing and pushing images to various image storages
* A CLI utility for interacting with OCID. Any CLIs built as part of this project are only meant for testing this project and there will be no guarantees on the backwards compatibility with it.

This is an implementation of the Kubernetes Container Runtime Interface (CRI) that will allow Kubernetes to directly launch and manage Open Container Initiative (OCI) containers.

The plan is to use OCI projects and best of breed libraries for different aspects:
- Runtime: [runc](https://github.com/opencontainers/runc) (or any OCI runtime-spec implementation) and [oci runtime tools](https://github.com/opencontainers/runtime-tools) 
- Images: Image management using [containers/image](https://github.com/containers/image)
- Storage: Storage and management of image layers using [containers/storage](https://github.com/containers/storage)
- Networking: Networking support through use of [CNI](https://github.com/containernetworking/cni)

It is currently in active development in the Kubernetes community through the [design proposal](https://github.com/kubernetes/kubernetes/pull/26788).  Questions and issues should be raised in the Kubernetes [sig-node Slack channel](https://kubernetes.slack.com/archives/sig-node).

## Current Roadmap 

1. Basic pod/container lifecycle, basic image pull (already works)
1. Support for tty handling and state management
1. Basic integration with kubelet once client side changes are ready
1. Support for log management, networking integration using CNI, pluggable image/storage management
1. Support for exec/attach
1. Target fully automated kubernetes testing without failures
