# CRI-O crictl Tutorial

This tutorial will walk you through the creation of [Redis](https://redis.io/)
server running in a [Pod](http://kubernetes.io/docs/user-guide/pods/) using
[crictl](https://github.com/kubernetes-sigs/cri-tools/blob/master/docs/crictl.md)

It assumes you've already downloaded and configured `CRI-O`. If not, see
[here for CRI-O](/install.md).
It also assumes you've set up CNI, and are using the default plugins as described
[here](/contrib/cni/README.md). If you are using a different configuration,
results may vary.

## Installation

This section will walk you through installing the following components:

- crictl - The CRI client for testing.

### Get crictl

See [install-crictl](https://github.com/kubernetes-sigs/cri-tools#install) for details
on how to install crictl.

### Ensure the CRI-O service is running

```shell
sudo crictl --runtime-endpoint unix:///var/run/crio/crio.sock version
```

```text
Version:  0.1.0
RuntimeName:  cri-o
RuntimeVersion:  1.20.0-dev
RuntimeApiVersion:  v1alpha1
```

> to avoid setting `--runtime-endpoint` when calling crictl,
> you can run `export CONTAINER_RUNTIME_ENDPOINT=unix:///var/run/crio/crio.sock`
> or `cp crictl.yaml /etc/crictl.yaml` from this repo

## Pod Tutorial

Now that the `CRI-O` components have been installed and configured you are ready
to create a Pod. This section will walk you through launching a Redis server
in a Pod. Once the Redis server is running we'll use telnet to verify it's working,
then we'll stop the Redis server and clean up the Pod.

### Creating a Pod

First we need to setup a Pod sandbox using a Pod configuration, which can be found
in the `CRI-O` source tree:

```shell
cd $GOPATH/src/github.com/cri-o/cri-o
```

In case the file `/etc/containers/policy.json` does not exist on your filesystem,
make sure that Skopeo has been installed correctly. You can use a policy template
provided in the CRI-O source tree, but it is insecure and it is not to be used
on production machines:

```shell
sudo mkdir /etc/containers/
sudo cp test/policy.json /etc/containers
```

Next create the Pod and capture the Pod ID for later use:

```shell
POD_ID=$(sudo crictl runp test/testdata/sandbox_config.json)
```

Use the `crictl` command to get the status of the Pod:

```shell
sudo crictl inspectp --output table $POD_ID
```

Output:

```text
ID: 3cf919ba84af36642e6cdb55e157a62407dec99d3cd319f46dd8883163048330
Name: podsandbox1
UID: redhat-test-crio
Namespace: redhat.test.crio
Attempt: 1
Status: SANDBOX_READY
Created: 2020-11-12 12:53:41.345961219 +0100 CET
IP Addresses: 10.85.0.7
Labels:
  group -> test
  io.kubernetes.container.name -> POD
Annotations:
  owner -> hmeng
  security.alpha.kubernetes.io/seccomp/pod -> unconfined
Info: # Redacted
```

### Create a Redis container inside the Pod

Use the `crictl` command to pull the Redis image.

```shell
sudo crictl pull quay.io/crio/fedora-crio-ci:latest
```

Create a Redis container from
a container configuration and attach it to the Pod created earlier,
while capturing the container ID:

```shell
CONTAINER_ID=$(sudo crictl create $POD_ID test/testdata/container_redis.json test/testdata/sandbox_config.json)
```

The `crictl create` command will take a few seconds to return because the Redis
container needs to be pulled.

Start the Redis container:

```shell
sudo crictl start $CONTAINER_ID
```

Get the status for the Redis container:

```shell
sudo crictl inspect $CONTAINER_ID
```

Output:

```text
ID: f70e2a71239c6724a897da98ffafdfa4ad850944098680b82d381d757f4bcbe1
Name: podsandbox1-redis
State: CONTAINER_RUNNING
Created: 32 seconds ago
Started: 16 seconds ago
Labels:
  tier -> backend
Annotations:
  pod -> podsandbox1
Info: # Redacted
```

### Test the Redis container

Fetch the Pod IP (can also be obtained via the `inspectp` output above):

<!-- markdownlint-disable MD013 -->

```shell
POD_IP=$(sudo crictl inspectp --output go-template --template '{{.status.network.ip}}' $POD_ID)
```

<!-- markdownlint-enable MD013 -->

Verify the Redis server is responding to `MONITOR` commands:

```shell
echo MONITOR | ncat $POD_IP 6379
```

Output:

```text
+OK
```

#### Viewing the Redis logs

The Redis logs are logged to the stderr of the crio service,
which can be viewed using `journalctl`:

```shell
sudo journalctl -u crio --no-pager
```

### Stop and delete the Redis container

```shell
sudo crictl stop $CONTAINER_ID
sudo crictl rm $CONTAINER_ID
```

Verify the container is gone via:

```shell
sudo crictl ps
```

### Stop and delete the Pod

```shell
sudo crictl stopp $POD_ID
sudo crictl rmp $POD_ID
```

Verify the pod is gone via:

```shell
sudo crictl pods
```
