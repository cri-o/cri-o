# Container Runtime Interface special cases

The target of this document is to outline corner cases and common pitfalls in
conjunction with the Container Runtime Interface (CRI). This document outlines
CRI-O's interpretation of certain aspects of the interface, which may not be
completely formalized.

The main documentation of the CRI can be found [in the corresponding protobuf
definition][0], whereas this document follows it on the `service`/`rpc` level.

## `ListImages`

`ListImages` lists existing images. Its response consists of an array of
`Image` types. Besides other information, an `Image` contains `repo_tags` and
`repo_digests`, which are defined as:

```proto
// Other names by which this image is known.
repeated string repo_tags = 2;

// Digests by which this image is known.
repeated string repo_digests = 3;
```

Both tags and digests will be used by:

- The kubelet, which displays them in the node status as a flat list, for example:

  ```shell
  kubectl get node 127.0.0.1 -o json | jq .status.images
  ```

  ```json
  [
    {
      "names": [
        "registry.k8s.io/pause@sha256:4a1c4b21597c1b4415bdbecb28a3296c6b5e23ca4f9feeb599860a1dac6a0108",
        "registry.k8s.io/pause@sha256:927d98197ec1141a368550822d18fa1c60bdae27b78b0c004f705f548c07814f",
        "registry.k8s.io/pause:3.2"
      ],
      "sizeBytes": 688049
    }
  ]
  ```

  Right now, the amount of images shown is limited by the kubelet flag
  `--node-status-max-images` (). The scheduler uses this list to
  score nodes based on the information if a container image already exists.

- crictl, which is able to output the image list in a human readable way:

  ```shell
  sudo crictl images --digests
  IMAGE                 TAG       DIGEST           IMAGE ID         SIZE
  registry.k8s.io/pause      3.2       4a1c4b21597c1    80d28bedfe5de    688kB
  ```

CRI-O implements the [`ConvertImage`][1] function to follow the following self-defined
rules:

- always return at least one `repo_digests` value
- return zero or more `repo_tags` values

There are multiple use-cases where this behavior is relevant. Those will be
covered separately by using real world examples.

### Pulling an image from a remote registry

This is the standard behavior and already shown in the `pause` image example
above. `crictl` is able to display all information, like the image name, tag and
digest. There are multiple digests available for this image, which gets a
correct representation within the `kubelet`'s node status.

### Pulling an updated version of an image with the same tag

Let's assume we pulled the image `quay.io/saschagrunert/hello-world` and
afterwards its `latest` tag got updated. Now we pull the image again, which
results in untagging the local image in favor of the new remote one.

CRI-O would now have no available `RepoTags` nor `RepoDigests` within the
`storage.ImageResult`. In this case, CRI-O uses an assembled `repoDigests`
value from the `PreviousName` and the image digest:

```go
repoDigests = []string{from.PreviousName + "@" + string(from.Digest)}
```

This allows tools like `crictl` to output the image name by adding a `<none>`
placeholder for the tag:

```shell
sudo crictl images --digests
```

```text
IMAGE                               TAG       DIGEST           IMAGE ID         SIZE
quay.io/saschagrunert/hello-world   <none>    2403474085c1e    14c28051b743c    5.88MB
quay.io/saschagrunert/hello-world   latest    ca810c5740f66    d1165f2212346    17.7kB
```

The `kubelet` is still able to list the image by its digest, which could be
referenced by a Kubernetes container:

```shell
kubectl get node 127.0.0.1 -o json | jq .status.images
```

```json
{
  "names": [
    "quay.io/saschagrunert/hello-world@sha256:2403474085c1e68c0aa171eb1b2b824a841a4aa636a4f2500c8d2e2f6d3cb422"
  ],
  "sizeBytes": 5884835
}
```

#### Building container images locally

We assume that we consecutively build a container image locally like this:

```shell
sudo podman build --no-cache -t test .
```

The previous image tag gets removed by Podman and applied to the current build.
In that case CRI-O will use the `PreviousName` in the same way as described in
the use-case above.

#### Pulling images by digest

If we pull a container image by its digest like this:

```shell
sudo crictl pull docker.io/alpine@sha256:2a8831c57b2e2cb2cda0f3a7c260d3b6c51ad04daea0b3bfc5b55f489ebafd71
```

Then CRI-O will not be able to provide a `RepoTags` result, but a single entry
in `RepoDigests`. The output for tools like `crictl` will be the same as
described in the examples above. In the same way the node status receives the
single digest entry:

```json
{
  "names": [
    "docker.io/library/alpine@sha256:2a8831c57b2e2cb2cda0f3a7c260d3b6c51ad04daea0b3bfc5b55f489ebafd71"
  ],
  "sizeBytes": 5850080
}
```

[0]: https://github.com/kubernetes/cri-api/blob/ca4df7a/pkg/apis/runtime/v1/api.proto
[1]: https://github.com/cri-o/cri-o/blob/main/server/image_list.go#L31
