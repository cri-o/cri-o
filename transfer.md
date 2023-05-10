# Introduction to CRI-O and Podman

CRI-O is a lightweight runtime for Kubernetes that provides the CRI
(Container Runtime Interface) socket required for automating the deployment,
scaling, and management of containerized applications. It serves as an
alternative to Docker within Kubernetes environments. However, it's important
to note that CRI-O is not a drop-in replacement for Docker, and there are
some differences in functionality and tooling.

## Understanding the Container Runtime Interface (CRI)

The CRI is a standardized interface between Kubernetes and container runtimes,
allowing Kubernetes to manage and interact with containers. It defines a set of
operations and APIs that Kubernetes uses to create, start, stop, and delete
containers. CRI-O implements this interface and provides the necessary
functionalities for Kubernetes to work seamlessly.

## CRI-O Operational Considerations

When transitioning from Docker to CRI-O, it's crucial to understand that many
traditional Docker commands and tools may not directly apply to CRI-O.
While some equivalents exist, such as `crictl` (a command-line utility that
serves as a client for the Container Runtime Interface (CRI)), they are
primarily focused on fulfilling the requirements of the Kubernetes CRI.

For operational tasks and troubleshooting within a Kubernetes environment, it is
recommended to leverage additional tools like [Podman](https://github.com/containers/podman).
These tools offer a feature-rich set of commands that can address various
operational needs. However, it's important to note that direct interaction
with CRI-O containers using Podman commands is not possible. While images
can be shared between Podman and CRI-O, containers themselves cannot be
directly managed or interacted with across these tools. To interact with
CRI-O containers, you should use tools that interface with the CRI, such as `crictl`.

### System Tools

Many traditional tools will still be useful, such as `pstree`, `nsenter` and `lsns`.
As well as some systemd helpers like `systemd-cgls` and
`systemd-cgtop` are still just as applicable.

## Podman as an Alternative for Debugging

If you are primarily interested in debugging containers and require a tool that
offers extensive command-line capabilities, Podman is a viable alternative.
Podman is a daemonless container engine that provides a command-line interface
similar to Docker. It can run containers, manage container images, and perform
various container-related operations.

While Podman and CRI-O are separate projects with different purposes, Podman offers
a more comprehensive set of commands that can facilitate debugging and
troubleshooting tasks within a containerized environment. You can use Podman
commands to perform actions like executing commands within a container (`podman exec`),
inspecting container metadata (`podman inspect`),
viewing container logs (`podman logs`), and many others.

It's important to note that Podman and CRI-O are not interchangeable. Podman is
a standalone container engine that operates independently of Kubernetes, while
CRI-O is specifically designed for Kubernetes environments. However, Podman can
be a valuable tool when it comes to container debugging and development workflows.

### Equivalents

For many troubleshooting and information collection steps, there may be an
existing pattern. Following provides equivalent with CRI-O tools for gathering
information or jumping into containers, for operational use.

| Existing Step    | CRI-O (and friends)                          |
|:----------------:|:--------------------------------------------:|
| `docker exec`    | [`crictl exec`][crictl]                      |
| `docker inspect` | `podman inspect`                             |
| `docker logs`    | `podman logs`                                |
| `docker ps`      | [`crictl ps`][crictl] or [`runc list`][runc] |
| `docker stats`   | `podman stats`                               |

[crictl]: https://github.com/kubernetes-sigs/cri-tools/blob/master/docs/crictl.md
[runc]: https://github.com/opencontainers/runc/blob/main/man/runc-list.8.md

If you were already using steps like `kubectl exec` (or `oc exec` on OpenShift),
they will continue to function the same way.

## Conclusion

In summary, CRI-O is a lightweight runtime that implements the CRI interface for
Kubernetes, providing container management capabilities within a Kubernetes
environment. While it's not a direct replacement for Docker, it offers
compatibility and integration with Kubernetes. For operational tasks, it is
recommended to utilize additional tools like Podman, which provides a more
extensive command-line interface for container debugging and troubleshooting.
