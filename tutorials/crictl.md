# CRI-O crictl Tutorial

This tutorial will walk you through the creation of [Redis](https://redis.io/) server running in a [Pod](http://kubernetes.io/docs/user-guide/pods/) using [crictl](https://github.com/kubernetes-sigs/cri-tools/blob/master/docs/crictl.md)

It assumes you've already downloaded and configured `CRI-O`. If not, see [here for CRI-O](setup.md).
It also assumes you've set up CNI, and are using the default plugins as described [here](contrib/cni/README.md). If you are using a different configuration, results may vary.

## Installation

This section will walk you through installing the following components:

* crictl - The CRI client for testing.

#### Get crictl

```
go get github.com/kubernetes-sigs/cri-tools/cmd/crictl
```

#### Ensure the CRI-O service is running

```
sudo crictl --runtime-endpoint unix:///var/run/crio/crio.sock version
```
```
Version:  0.1.0
RuntimeName:  CRI-O
RuntimeVersion:  1.10.0-dev
RuntimeApiVersion:  v1alpha1
```

> to avoid setting `--runtime-endpoint` when calling crictl,
> you can run `export CONTAINER_RUNTIME_ENDPOINT=unix:///var/run/crio/crio.sock`
> or `cp crictl.yaml /etc/crictl.yaml` from this repo


## Pod Tutorial

Now that the `CRI-O` components have been installed and configured we are ready to create a Pod. This section will walk you through launching a Redis server in a Pod. Once the Redis server is running we'll use telnet to verify it's working, then we'll stop the Redis server and clean up the Pod.

### Creating a Pod

First we need to setup a Pod sandbox using a Pod configuration, which can be found in the `CRI-O` source tree:

```
cd $GOPATH/src/github.com/cri-o/cri-o
```

In case the file `/etc/containers/policy.json` does not exist on your filesystem, make sure that skopeo has been installed correctly. You can use a policy template provided in the CRI-O source tree, but it is insecure and it is not to be used on production machines:

```
sudo mkdir /etc/containers/
sudo cp test/policy.json /etc/containers
```


Next create the Pod and capture the Pod ID for later use:

```
POD_ID=$(sudo crictl runp test/testdata/sandbox_config.json)
```


Use the `crictl` command to get the status of the Pod:

```
sudo crictl inspectp --output table $POD_ID
```

Output:

```
ID: cd6c0883663c6f4f99697aaa15af8219e351e03696bd866bc3ac055ef289702a
Name: podsandbox1
UID: redhat-test-crio
Namespace: redhat.test.crio
Attempt: 1
Status: SANDBOX_READY
Created: 2016-12-14 15:59:04.373680832 +0000 UTC
Network namespace: /var/run/netns/cni-bc37b858-fb4d-41e6-58b0-9905d0ba23f8
IP Address: 10.88.0.2
Labels:
	group -> test
Annotations:
	owner -> hmeng
	security.alpha.kubernetes.io/seccomp/pod -> unconfined
	security.alpha.kubernetes.io/sysctls -> kernel.shm_rmid_forced=1,net.ipv4.ip_local_port_range=1024 65000
	security.alpha.kubernetes.io/unsafe-sysctls -> kernel.msgmax=8192
```

### Create a Redis container inside the Pod

Use the `crictl` command to pull the Redis image, create a Redis container from a container configuration and attach it to the Pod created earlier, while capturing the container ID:

```
sudo crictl pull quay.io/crio/redis:alpine
CONTAINER_ID=$(sudo crictl create $POD_ID test/testdata/container_redis.json test/testdata/sandbox_config.json)
```


The `crictl create` command  will take a few seconds to return because the Redis container needs to be pulled.

Start the Redis container:

```
sudo crictl start $CONTAINER_ID
```

Get the status for the Redis container:

```
sudo crictl inspect $CONTAINER_ID
```

Output:

```
ID: d0147eb67968d81aaddbccc46cf1030211774b5280fad35bce2fdb0a507a2e7a
Name: podsandbox1-redis
Status: CONTAINER_RUNNING
Created: 2016-12-14 16:00:42.889089352 +0000 UTC
Started: 2016-12-14 16:01:56.733704267 +0000 UTC
```

### Test the Redis container

Connect to the Pod IP on port 6379:

```
telnet 10.88.0.2 6379
```

```
Trying 10.88.0.2...
Connected to 10.88.0.2.
Escape character is '^]'.
```

At the prompt type `MONITOR`:

```
Trying 10.88.0.2...
Connected to 10.88.0.2.
Escape character is '^]'.
MONITOR
+OK
```

Exit the telnet session by typing `ctrl-]` and `quit` at the prompt:

```
^]

telnet> quit
Connection closed.
```

#### Viewing the Redis logs

The Redis logs are logged to the stderr of the crio service, which can be viewed using `journalctl`:

```
sudo journalctl -u crio --no-pager
```

### Stop the Redis container and delete the Pod

```
sudo crictl stop $CONTAINER_ID
```

```
sudo crictl rm $CONTAINER_ID
```

```
sudo crictl stopp $POD_ID
```

```
sudo crictl rmp $POD_ID
```

```
sudo crictl pods
```

```
sudo crictl ps
```
