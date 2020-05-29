# CRI-O Metrics

To enable the [Prometheus][0] metrics exporter for CRI-O, either start `crio`
with `--metrics-enable` or add the corresponding option to a config overwrite,
for example `/etc/crio/crio.conf.d/01-metrics.conf`:

```toml
[crio.metrics]
enable_metrics = true
```

The metrics endpoint serves per default on port `9090`. This can be changed via
the `--metrics-port` command line argument or via the configuration file:

```toml
metrics_port = 9090
```

If CRI-O runs with enabled metrics, then this can be verified by querying the
endpoint manually via [curl][1].

```bash
> curl localhost:9090/metrics
…
```

## Available Metrics

Beside the [default golang based metrics][2], CRI-O provides the following additional metrics:

| Metric Key                             | Possible Labels                                                                                                                         | Type    | Purpose                                                                  |
| -------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------- | ------- | ------------------------------------------------------------------------ |
| `crio_operations`                      | every CRI-O RPC\*                                                                                                                       | Counter | Cumulative number of CRI-O operations by operation type.                 |
| `crio_operations_latency_microseconds` | every CRI-O RPC\*,<br><br>`network_setup_pod` (CNI pod network setup time),<br><br>`network_setup_overall` (Overall network setup time) | Summary | Latency in microseconds of CRI-O operations. Split-up by operation type. |
| `crio_operations_errors`               | every CRI-O RPC\*                                                                                                                       | Counter | Cumulative number of CRI-O operation errors by operation type.           |
| `crio_image_pulls_by_digest`           | `name`, `digest`, `mediatype`, `size`                                                                                                   | Counter | Bytes transferred by CRI-O image pulls by digest.                        |
| `crio_image_pulls_by_name`             | `name`, `size`                                                                                                                          | Counter | Bytes transferred by CRI-O image pulls by name.                          |
| `crio_image_pulls_by_name_skipped`     | `name`                                                                                                                                  | Counter | Bytes skipped by CRI-O image pulls by name.                              |
| `crio_image_pulls_successes`           | `name`                                                                                                                                  | Counter | Successful image pulls by image name                                     |
| `crio_image_pulls_failures`            | `name`, `error`                                                                                                                         | Counter | Failed image pulls by image name and their error category.               |

- Available CRI-O RPC's from the [gRPC API][3]: `Attach`, `ContainerStats`, `ContainerStatus`,
  `CreateContainer`, `Exec`, `ExecSync`, `ImageFsInfo`, `ImageStatus`,
  `ListContainerStats`, `ListContainers`, `ListImages`, `ListPodSandbox`,
  `PodSandboxStatus`, `PortForward`, `PullImage`, `RemoveContainer`,
  `RemoveImage`, `RemovePodSandbox`, `ReopenContainerLog`, `RunPodSandbox`,
  `StartContainer`, `Status`, `StopContainer`, `StopPodSandbox`,
  `UpdateContainerResources`, `UpdateRuntimeConfig`, `Version`

- Available error categories for `crio_image_pulls_failures`:
  - `UNKNOWN`: The default label which gets applied if the error is not known
  - `CONNECTION_REFUSED`: The local network is down or the registry refused the
    connection.
  - `CONNECTION_TIMEOUT`: The connection timed out during the image download.
  - `NOT_FOUND`: The registry does not exist at the specified resource
  - `BLOB_UNKNOWN`: This error may be returned when a blob is unknown to the
    registry in a specified repository. This can be returned with a standard get
    or if a manifest references an unknown layer during upload.
  - `BLOB_UPLOAD_INVALID`: The blob upload encountered an error and can no
    longer proceed.
  - `BLOB_UPLOAD_UNKNOWN`: If a blob upload has been cancelled or was never
    started, this error code may be returned.
  - `DENIED`: The access controller denied access for the operation on a
    resource.
  - `DIGEST_INVALID`: When a blob is uploaded, the registry will check that the
    content matches the digest provided by the client. The error may include a
    detail structure with the key "digest", including the invalid digest string.
    This error may also be returned when a manifest includes an invalid layer
    digest.
  - `MANIFEST_BLOB_UNKNOWN`: This error may be returned when a manifest blob is
    unknown to the registry.
  - `MANIFEST_INVALID`: During upload, manifests undergo several checks ensuring
    validity. If those checks fail, this error may be returned, unless a more
    specific error is included. The detail will contain information the failed
    validation.
  - `MANIFEST_UNKNOWN`: This error is returned when the manifest, identified by
    name and tag is unknown to the repository.
  - `MANIFEST_UNVERIFIED`: During manifest upload, if the manifest fails
    signature verification, this error will be returned.
  - `NAME_INVALID`: Invalid repository name encountered either during manifest.
    validation or any API operation.
  - `NAME_UNKNOWN`: This is returned if the name used during an operation is
    unknown to the registry.
  - `SIZE_INVALID`: When a layer is uploaded, the provided size will be checked
    against the uploaded content. If they do not match, this error will be
    returned.
  - `TAG_INVALID`: During a manifest upload, if the tag in the manifest does not
    match the uri tag, this error will be returned.
  - `TOOMANYREQUESTS`: Returned when a client attempts to contact a service too
    many times.
  - `UNAUTHORIZED`: The access controller was unable to authenticate the client.
    Often this will be accompanied by a Www-Authenticate HTTP response header
    indicating how to authenticate.
  - `UNAVAILABLE`: Returned when a service is not available.
  - `UNSUPPORTED`: The operation was unsupported due to a missing implementation
    or invalid set of parameters.

[0]: https://prometheus.io
[1]: https://github.com/curl/curl
[2]: https://github.com/prometheus/client_golang/blob/b8b56b52bdb3a79ab877c873463cadc841133360/prometheus/go_collector.go#L65-L281
[3]: https://github.com/kubernetes/cri-api/blob/a6f63f369f6d50e9d0886f2eda63d585fbd1ab6a/pkg/apis/runtime/v1alpha2/api.proto#L34-L128
