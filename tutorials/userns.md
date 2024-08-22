# User namespaces and CRI-O

The purpose of this document is to show how to configure user namespaces in CRI-O,
as well as some of the options CRI-O supports for configuring user namespaces.

## Setup

### /etc/sub{g,u}id

To start, the host will have to have `/etc/subuid` and `/etc/subgid` files set correctly.
By default, the [library CRI-O uses for container storage](https://github.com/containers/storage)
assumes there will be entries in each of these files for the `containers` user.
If one would like to have a different user's entries in `/etc/sub?id` files,
then the field `remap-user` and `remap-group` can be configured in
`/etc/containers/storage.conf` in the `[storage.options]` table.

Let's assume we want the IDs of the users and groups to begin on the host at 100000,
and be each given ranges of 65536. For most containers, this will be more than enough.
The contents of both `/etc/subuid` and `/etc/subgid` should be:

```text
containers:100000:65536
```

### CRI-O configuration

To enable pods to be able to use the userns-mode annotation, the pod must be
allowed to interpret the experimental annotation `io.kubernetes.cri-o.userns-mode`.

#### 1.23.0 and beyond

In CRI-O versions greater than 1.23.0, this can be done by creating a custom workload.
This can be done by creating a file with the following contents in /etc/crio/crio.conf.d/01-userns-workload.conf

```toml
[crio.runtime.workloads.userns]
activation_annotation = "io.kubernetes.cri-o.userns-mode"
allowed_annotations = ["io.kubernetes.cri-o.userns-mode"]
```

This will allow any pod with the `io.kubernetes.cri-o.userns-mode` annotation to
configure a user namespace. CRI-O opts for this approach to give administrators
the ability to toggle the behavior on their nodes, just in case an administrator
doesn't want their users to be able to create user namespace. An administrator
can also set a different `activation_annotation` if they'd like a
different annotation to allow pods to configure user namespaces.

#### 1.20.0 - 1.22.z

CRI-O has supported this experimental annotation since 1.20.0. Originally, it was
supported by setting allowed annotations in the runtime class, not the workload.
Setting allowed_annotations on runtimes have been deprecated, and newer installations
should use workloads instead. To create a runtime class that allows the user namespace
annotation, the following file can be created:

```toml
[crio.runtime.runtimes.userns]
runtime_path = "/usr/bin/runc"
runtime_root = "/run/runc"
allowed_annotations = ["io.kubernetes.cri-o.userns-mode"]
```

`runtime_path` and `runtime_root` can be configured differently, but must be specified.

The name `userns` will be the one that must be specified in the pod's `runtimeClassName`
field. See [this article](https://kubernetes.io/docs/concepts/containers/runtime-class/)
for more details. The remainder of this document will
assume workloads are being used.

### Pod Spec Changes

Now that the pod is allowed to specify the annotation, it must actually be done
in the pod spec. We will use the simplest example "auto" for this pod spec:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: mypod
  annotations:
    io.kubernetes.cri-o.userns-mode: "auto"
```

In this case, upon pod creation, the pod will have a user namespace automatically
configured for it. With a user on the host that is
greater than 100000 and with a size of 65536.

## User Namespace Configuration

### Auto

The auto keyword tells CRI-O that a user namespace should be configured for the
pod by CRI-O. This is a good option for users who are new to user namespaces,
or don't have precise needs for the feature.

#### RunAs{User,Group} and User Namespaces

When RunAsUser or RunAsGroup are specified for a container in a pod, and the user
namespace mode is "auto", the user namespace is configured to have that user
inside of the user namespace, but the user in the host user namespace is in the
range configured in `/etc/subuid`.

For instance, in RunAsUser is set to `1234` for a pod that specifies auto along
with the `/etc/subuid` configuration above, the pod user inside the pod sees
itself as `1234`. However, from the perspective of the host, the pod user
could actually be `101234`.

This allows for the container process to think it is running as user 1234 for
file access inside of the container, but actually be a much higher ID on the host.
