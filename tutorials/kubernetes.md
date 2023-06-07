# Running CRI-O on a Kubernetes cluster

## Switching runtime from Docker to CRI-O

In a standard Docker Kubernetes cluster, kubelet is running on each node as systemd
service and is taking care of communication between runtime and the API service.
It is responsible for starting microservices pods (such as `kube-proxy`, `kubedns`,
etc. - they can be different for various ways of deploying k8s) and user pods.
Configuration of kubelet determines which runtime is used and in what way.

Kubelet itself is executed in Docker container (as we can see in `kubelet.service`),
but, what is important, **it's not** a Kubernetes pod (at least for now),
so we can keep kubelet running inside the container (as well as directly on the host),
and regardless of this, run pods in the chosen runtime.

Below, you can find an instruction how to switch one or more nodes on running
Kubernetes cluster from Docker to CRI-O.

### Preparing kubelet

At first, you need to stop kubelet service working on the node:

```shell
systemctl stop kubelet
```

and stop all kubelet Docker containers that are still running.

```shell
docker stop $(docker ps | grep k8s_ | awk '{print $1}')
```

We have to be sure that `kubelet.service` will start after `crio.service`.
It can be done by adding `crio.service` to `Wants=` section in `/etc/systemd/system/kubelet.service`:

```shell
$ cat /etc/systemd/system/kubelet.service | grep Wants
Wants=docker.socket crio.service
```

If you'd like to change the way of starting kubelet (e.g., directly on the host instead
of in a container), you can change it here, but, as mentioned, it's not necessary.

Kubelet parameters are stored in `/etc/kubernetes/kubelet.env` file.

```shell
$ cat /etc/kubernetes/kubelet.env | grep KUBELET_ARGS
KUBELET_ARGS="--pod-manifest-path=/etc/kubernetes/manifests
--pod-infra-container-image=gcr.io/google_containers/pause-amd64:3.0
--cluster_dns=10.233.0.3 --cluster_domain=cluster.local
--resolv-conf=/etc/resolv.conf --kubeconfig=/etc/kubernetes/node-kubeconfig.yaml
--require-kubeconfig"
```

You need to add following parameters to `KUBELET_ARGS`:

- `--container-runtime-endpoint=unix:///var/run/crio/crio.sock`

Socket for remote runtime (default `crio` socket localization).

- `--runtime-request-timeout=10m` - Optional but useful.
  Some requests, especially pulling huge images, may take longer than
  default (2 minutes) and will cause an error.

You may need to add following parameter to `KUBELET_ARGS` (prior to Kubernetes
`v1.24.0-alpha.2`). This flag is deprecated since `v1.24.0-alpha.2`, and will no
longer be available starting from `v1.27.0`:

- `--container-runtime=remote` - Use remote runtime with provided socket.

Kubelet is prepared now.

## Flannel network

If your cluster is using flannel network, your network configuration should be like:

```shell
$ cat /etc/cni/net.d/10-crio.conf
{
    "name": "crio",
    "type": "flannel"
}
```

Then, kubelet will take parameters from `/run/flannel/subnet.env` - file generated
by flannel kubelet microservice.

## Starting kubelet with CRI-O

Start crio first, then kubelet. If you created `crio` service:

```shell
systemctl start crio
systemctl start kubelet
```

You can follow the progress of preparing node using `kubectl get nodes` or
`kubectl get pods --all-namespaces` on Kubernetes control-plane.
