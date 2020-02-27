# Running CRI-O with kubeadm

This tutorial assumes you've already installed and setup CRI-O. If you have not, start [here](setup.md).
It also assumes you've set up your system to use kubeadm. If you haven't done so, [here is a good tutorial](https://www.mirantis.com/blog/how-install-kubernetes-kubeadm/)

### Configuring CNI

First, CRI-O and `kubeadm reset` don't play well together, as `kubeadm reset` empties `/etc/cni/net.d/`.
Therefore, it is good to add an overwrite of the `crio.network.network_dir` in `/etc/crio/crio.conf.d/01-overwrite-cni.conf` to somewhere kubeadm won't touch.

Further, you'll need to use your plugins to figure out your pod-network-cidr. If you use the default bridge plugin defined [here](/contrib/cni/10-crio-bridge.conf), set
```CIDR=10.88.0.0/16```
If you're using a flannel network, set
```CIDR=10.244.0.0/16```

# Configuring kubelet

There is a handy, preprepared `/etc/default/kubelet` file [here](https://gist.githubusercontent.com/haircommander/2c07cc23887fa7c7f083dc61c7ef5791/raw/73e3d27dcd57e7de237c08758f76e0a368547648/cri-o-kubeadm)
This will configure kubeadm to run with the correct defaults CRI-O needs to run.

Note: This file assumes you've set your cgroup_driver as systemd

# Running kubeadm

Given you've set CIDR, and you've properly set the kubelet file, all you need to do is start crio (as defined [here](setup.md)), and run:
`kubeadm --pod-network-cidr=$CIDR`
