# CRI-O Usage Transfer

This document outlines useful information for operations and development transfer as it relates to infrastructure that utilizes CRI-O.

## Operational Transfer

## Abstract

The `crio` daemon is intended to provide the CRI socket needed for Kubernetes to use for automating deployment, scaling, and management of containerized applications (See the document for [configuring kubernetes to use CRI-O](./tutorials/kubernetes.md) for more information).
Therefore the [crictl][1] command line is a client that interfaces to the same gRPC socket as the kubernetes daemon would, for talking to the `crio` daemon.
In many ways [crictl][1] is only as feature rich as the Kubernetes CRI requires.
There are additional tools e.g. [Podman](https://github.com/containers/podman) and [Buildah](https://github.com/projectatomic/buildah) that provide a feature rich set of commands for all operational needs in a Kubernetes environment.

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
| `docker attach` | [`podman exec`](https://podman.readthedocs.io/en/latest/markdown/podman-exec.1.html) ***|
| `docker build`  | [`buildah bud`](https://github.com/projectatomic/buildah/blob/master/docs/buildah-bud.md) |
| `docker cp`     | [`podman mount`](https://podman.readthedocs.io/en/latest/markdown/podman-mount.1.html) ****   |
| `docker create` | [`podman create`](https://podman.readthedocs.io/en/latest/markdown/podman-create.1.html)  |
| `docker diff`   | [`podman diff`](https://podman.readthedocs.io/en/latest/markdown/podman-diff.1.html)      |
| `docker export` | [`podman export`](https://podman.readthedocs.io/en/latest/markdown/podman-export.1.html)  |
| `docker history`| [`podman history`](https://podman.readthedocs.io/en/latest/markdown/podman-history.1.html)|
| `docker images` | [`podman images`](https://podman.readthedocs.io/en/latest/markdown/podman-images.1.html)  |
| `docker kill`   | [`podman kill`](https://podman.readthedocs.io/en/latest/markdown/podman-kill.1.html)      |
| `docker load`   | [`podman load`](https://podman.readthedocs.io/en/latest/markdown/podman-load.1.html)      |
| `docker login`  | [`podman login`](https://podman.readthedocs.io/en/latest/markdown/podman-login.1.html)    |
| `docker logout` | [`podman logout`](https://podman.readthedocs.io/en/latest/markdown/podman-logout.1.html)  |
| `docker pause`  | [`podman pause`](https://podman.readthedocs.io/en/latest/markdown/podman-pause.1.html)    |
| `docker ps`     | [`podman ps`](https://podman.readthedocs.io/en/latest/markdown/podman-ps.1.html)          |
| `docker pull`   | [`podman pull`](https://podman.readthedocs.io/en/latest/markdown/podman-pull.1.html)      |
| `docker push`   | [`podman push`](https://podman.readthedocs.io/en/latest/markdown/podman-push.1.html)      |
| `docker rename` | [`podman rename`](./docs/podman-rename.1.md)  |
| `docker rm`     | [`podman rm`](https://podman.readthedocs.io/en/latest/markdown/podman-rm.1.html)          |
| `docker rmi`    | [`podman rmi`](https://podman.readthedocs.io/en/latest/markdown/podman-rmi.1.html)        |
| `docker run`    | [`podman run`](https://podman.readthedocs.io/en/latest/markdown/podman-run.1.html)        |
| `docker save`   | [`podman save`](https://podman.readthedocs.io/en/latest/markdown/podman-save.1.html)      |
| `docker stop`   | [`podman stop`](https://podman.readthedocs.io/en/latest/markdown/podman-stop.1.html)      |
| `docker tag`    | [`podman tag`](https://podman.readthedocs.io/en/latest/markdown/podman-tag.1.html)        |
| `docker unpause`| [`podman unpause`](https://podman.readthedocs.io/en/latest/markdown/podman-unpause.1.html)|
| `docker version`| [`podman version`](https://podman.readthedocs.io/en/latest/markdown/podman-version.1.html)|
| `docker wait`   | [`podman wait`](https://podman.readthedocs.io/en/latest/markdown/podman-wait.1.html)   |

*** Use `podman exec` to enter a container and `podman logs` to view the output of pid 1 of a container.
**** Use mount to take advantage of the entire linux tool chain rather then just cp.  Read [`here`](https://podman.readthedocs.io/en/latest/markdown/podman-cp.1.html) for more information.
