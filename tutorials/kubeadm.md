# Running CRI-O with kubeadm

This tutorial assumes you've already installed and setup CRI-O.
If you have not, start [here](/install.md).
It also assumes you've set up your system to use kubeadm.
If you haven't done so, [here is a good tutorial](https://www.mirantis.com/blog/how-install-kubernetes-kubeadm/)

## Configuring CNI

kubeadm expects a POD Network CIDR (`--pod-network-cidr`) to be defined when you
install the cluster. The value of `--pod-network-cidr` depends on which
CNI plugin you choose.

<!-- markdownlint-disable MD013 -->
| CNI Plugin              | CIDR          | Notes                                                                                                                                         |
| ----------------------- | ------------- | --------------                                                                                                                                |
| Bridge plugin (default) | 10.85.0.0/16  | The default bridge plugin is defined [here](/contrib/cni/10-crio-bridge.conflist). This is only suitable when your cluster has a **single node**. |
| Flannel                 | 10.244.0.0/16 | This is a good choice for clusters with multiple nodes.                                                                                       |
<!-- markdownlint-enable MD013 -->

For example, to use the script below with the **bridge** plugin, run `export CIDR=10.85.0.0/16`.

A list of CNI plugins can be found in the [Cluster Networking](https://kubernetes.io/docs/concepts/cluster-administration/networking/)
Kubernetes documentation. Each plugin will define its own default CIDR.

## Running kubeadm

Given you've set CIDR, and assuming you've set the `cgroup_driver` in your CRI-O
configuration as `systemd` (which is the default value), all you need to do is
start crio (as defined [here](/install.md)), and run:
`kubeadm init --pod-network-cidr=$CIDR --cri-socket=unix:///var/run/crio/crio.sock`

## Running kubeadm in an off line network

We will assume that the user has installed CRI-O and all necessary packages.
We will also assume that all necessary components are configured and everything
is working as expected. The user should have a private repo where the container
images are pushed. An example of container images for Kubernetes version 1.18.2:

```bash
$ kubeadm config images list --image-repository user.private.repo --kubernetes-version=v1.18.2
user.private.repo/kube-apiserver:v1.18.2
user.private.repo/kube-controller-manager:v1.18.2
user.private.repo/kube-scheduler:v1.18.2
user.private.repo/kube-proxy:v1.18.2
user.private.repo/pause:3.2
user.private.repo/etcd:3.4.3-0
user.private.repo/coredns:1.6.7
```

The user needs to configure the [registries.conf](https://github.com/containers/image/blob/main/docs/containers-registries.conf.5.md)
file.

Sample of configurations:

```bash
$ cat /etc/containers/registries.conf
unqualified-search-registries = ["user.private.repo"]

[[registry]]
prefix = "registry.k8s.io"
insecure = false
blocked = false
location = "registry.k8s.io"

[[registry.mirror]]
location = "user.private.repo"
```

Next the user should reload and restart the CRI-O service to load the configurations.
