# Running CRI-O with kubeadm

This tutorial assumes you've already installed and setup CRI-O. If you have not, start [here](/install.md).
It also assumes you've set up your system to use kubeadm. If you haven't done so, [here is a good tutorial](https://www.mirantis.com/blog/how-install-kubernetes-kubeadm/)

## Configuring CNI

You'll need to use your plugins to figure out your pod-network-cidr. If you use the default bridge plugin defined [here](/contrib/cni/10-crio-bridge.conf), set
```CIDR=10.85.0.0/16```
If you're using a flannel network, set
```CIDR=10.244.0.0/16```

## Configuring Kubelet

To configure the Kubelet, you can use the [yq](https://github.com/mikefarah/yq) tool, as is shown below.
You can also manually configure a kubeadm configuration.

Run the following stript, passing the location you'd like the kubeadm configuration to be as the first argument:

```bash
#!/bin/bash                                                                                                                                                                                                                                   
                                                                                                                                                                                                                                              
set -euo pipefail                                                                                                                                                                                                                             
                                                                                                                                                                                                                                              
KUBEADM_CONFIG="${1-/tmp/kubeadm.yaml}"                                                                                                                                                                                                              
echo "Printing to $KUBEADM_CONFIG"                                                                                                                                                                                                                   
                                                                                                                                                                                                                                              
if [ -d "$KUBEADM_CONFIG" ]; then                                                                                                                                                                                                                    
    echo "$KUBEADM_CONFIG is a directory!"                                                                                                                                                                                                           
    exit 1                                                                                                                                                                                                                                    
fi                                                                                                                                                                                                                                            
                                                                                                                                                                                                                                              
if [ ! -d $(dirname "$KUBEADM_CONFIG") ]; then                                                                                                                                                                                                       
    echo "please create directory $(dirname $KUBEADM_CONFIG)"                                                                                                                                                                                        
    exit 1                                                                                                                                                                                                                                    
fi                                                                                                                                                                                                                                            
                                                                                                                                                                                                                                              
if [ ! $(which yq) ]; then                                                                                                                                                                                                                    
    echo "please install yq"                                                                                                                                                                                                                  
    exit 1                                                                                                                                                                                                                                    
fi                                                                                                                                                                                                                                            
                                                                                                                                                                                                                                              
if [ ! $(which kubeadm) ]; then                                                                                                                                                                                                               
    echo "please install kubeadm"                                                                                                                                                                                                             
    exit 1                                                                                                                                                                                                                                    
fi                                                                                                                                                                                                                                            
                                                                                                                                                                                                                                              
kubeadm config print init-defaults --component-configs=KubeletConfiguration > "$KUBEADM_CONFIG"                                                                                                                                                      
yq -i eval 'select(.nodeRegistration.criSocket) |= .nodeRegistration.criSocket = "unix:///var/run/crio/crio.sock"' "$KUBEADM_CONFIG"
yq -i eval 'select(di == 1) |= .cgroupDriver = "systemd"' "$KUBEADM_CONFIG"
```

This will create a kubeadm configuration file that kubeadm will use to configure the Kubelet to be able to communicate with CRI-O.

Note: This file assumes you've set the `cgroup_driver` in your CRI-O configuration as `systemd`, which is the default value.

## Running kubeadm

Given you've set CIDR, and you've properly created your kubeadm configuration file, all you need to do is start crio (as defined [here](/install.md)), and run:
`kubeadm init --pod-network-cidr=$CIDR --config=$KUBEADM_CONFIG`

## Running kubeadm in an off line network

We will assume that the user has installed CRI-O and all necessary packages. We will also assume that all necessary components are configured and everything is working as expected. The user should have a private repo where the container images are pushed. An example of container images for Kubernetes version 1.18.2:

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

The user needs to configure the [registries.conf](https://github.com/containers/image/blob/master/docs/containers-registries.conf.5.md) file.

Sample of configurations:

```bash
$ cat /etc/containers/registries.conf
unqualified-search-registries = ["user.private.repo"]

[[registry]]
prefix = "k8s.gcr.io"
insecure = false
blocked = false
location = "k8s.gcr.io"

[[registry.mirror]]
location = "user.private.repo"
```

Next the user should reload and restart the CRI-O service to load the configurations.
