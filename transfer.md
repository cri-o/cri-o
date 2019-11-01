# CRI-O Usage Transfer

This document outlines useful information for operations and development transfer as it relates to infrastructure that utilizes CRI-O.

## Operational Transfer

## Abstract

The `crio` daemon is intended to provide the CRI socket needed for Kubernetes to use for automating deployment, scaling, and management of containerized applications (See the document for [configuring kubernetes to use CRI-O](./tutorials/kubernetes.md) for more information).
Therefore the [crictl][1] command line is a client that interfaces to the same gRPC socket as the kubernetes daemon would, for talking to the `crio` daemon.
In many ways [crictl][1] is only as feature rich as the Kubernetes CRI requires.
There are additional tools e.g. [Podman](https://github.com/containers/libpod) and [Buildah](https://github.com/projectatomic/buildah) that provide a feature rich set of commands for all operational needs in a Kubernetes environment.

Please note that the full interoperability between CRI-O and Podman cannot be
guaranteed at this time. For example, it is not possible to interact with CRI-O
containers via Podman commands. To do this, please use tools which interferes
with the CRI, like [crictl][1].

[1]: https://github.com/kubernetes-sigs/cri-tools

## System Tools

Many traditional tools will still be useful, such as `pstree`, `nsenter` and `lsns`.
As well as some systemd helpers like `systemd-cgls` and `systemd-cgtop` are still just as applicable.

## Equivalents

For many troubleshooting and information collection steps, there may be an existing pattern.
Following provides equivalent with CRI-O tools for gathering information or jumping into containers, for operational use.

| Existing Step | CRI-O (and friends) |
| :---: | :---: |
| `docker exec` | [`crictl exec`](https://github.com/kubernetes-incubator/cri-tools/blob/master/docs/crictl.md) |
| `docker info` | [`podman info`](./docs/podman-info.1.md)  |
| `docker inspect` | [`podman inspect`](./docs/podman-inspect.1.md)       |
| `docker logs` | [`podman logs`](./docs/podman-logs.1.md)                 |
| `docker ps` | [`crictl ps`](https://github.com/kubernetes-incubator/cri-tools/blob/master/docs/crictl.md) or [`runc list`](https://github.com/opencontainers/runc/blob/master/man/runc-list.8.md) |
| `docker stats` | [`podman stats`](./docs/podman-stats.1.md) |

If you were already using steps like `kubectl exec` (or `oc exec` on OpenShift), they will continue to function the same way.

## Development Transfer

There are other equivalents for these tools

| Existing Step | CRI-O (and friends) |
| :---: | :---: |
| `docker attach` | [`podman exec`](./docs/podman-attach.1.md) ***|
| `docker build`  | [`buildah bud`](https://github.com/projectatomic/buildah/blob/master/docs/buildah-bud.md) |
| `docker cp`     | [`podman mount`](./docs/podman-cp.1.md) ****   |
| `docker create` | [`podman create`](./docs/podman-create.1.md)  |
| `docker diff`   | [`podman diff`](./docs/podman-diff.1.md)      |
| `docker export` | [`podman export`](./docs/podman-export.1.md)  |
| `docker history`| [`podman history`](./docs/podman-history.1.md)|
| `docker images` | [`podman images`](./docs/podman-images.1.md)  |
| `docker kill`   | [`podman kill`](./docs/podman-kill.1.md)      |
| `docker load`   | [`podman load`](./docs/podman-load.1.md)      |
| `docker login`  | [`podman login`](./docs/podman-login.1.md)    |
| `docker logout` | [`podman logout`](./docs/podman-logout.1.md)  |
| `docker pause`  | [`podman pause`](./docs/podman-pause.1.md)    |
| `docker ps`     | [`podman ps`](./docs/podman-ps.1.md)          |
| `docker pull`   | [`podman pull`](./docs/podman-pull.1.md)      |
| `docker push`   | [`podman push`](./docs/podman-push.1.md)      |
| `docker rename` | [`podman rename`](./docs/podman-rename.1.md)  |
| `docker rm`     | [`podman rm`](./docs/podman-rm.1.md)          |
| `docker rmi`    | [`podman rmi`](./docs/podman-rmi.1.md)        |
| `docker run`    | [`podman run`](./docs/podman-run.1.md)        |
| `docker save`   | [`podman save`](./docs/podman-save.1.md)      |
| `docker stop`   | [`podman stop`](./docs/podman-stop.1.md)      |
| `docker tag`    | [`podman tag`](./docs/podman-tag.1.md)        |
| `docker unpause`| [`podman unpause`](./docs/podman-unpause.1.md)|
| `docker version`| [`podman version`](./docs/podman-version.1.md)|
| `docker wait`   | [`podman wait`](./docs/podman-wait.1.md)   |

*** Use `podman exec` to enter a container and `podman logs` to view the output of pid 1 of a container.
**** Use mount to take advantage of the entire linux tool chain rather then just cp.  Read [`here`](./docs/podman-cp.1.md) for more information.
