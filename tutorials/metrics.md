# CRI-O Metrics

To enable the [Prometheus][0] metrics exporter for CRI-O, either start `crio`
with `--metrics-enable` or set the corresponding option in
`/etc/crio/crio.conf`:

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

\* Available CRI-O RPC's from the [gRPC API][3]: `Attach`, `ContainerStats`, `ContainerStatus`,
`CreateContainer`, `Exec`, `ExecSync`, `ImageFsInfo`, `ImageStatus`,
`ListContainerStats`, `ListContainers`, `ListImages`, `ListPodSandbox`,
`PodSandboxStatus`, `PortForward`, `PullImage`, `RemoveContainer`,
`RemoveImage`, `RemovePodSandbox`, `ReopenContainerLog`, `RunPodSandbox`,
`StartContainer`, `Status`, `StopContainer`, `StopPodSandbox`,
`UpdateContainerResources`, `UpdateRuntimeConfig`, `Version`

[0]: https://prometheus.io
[1]: https://github.com/curl/curl
[2]: https://github.com/prometheus/client_golang/blob/b8b56b52bdb3a79ab877c873463cadc841133360/prometheus/go_collector.go#L65-L281
[3]: https://github.com/kubernetes/cri-api/blob/a6f63f369f6d50e9d0886f2eda63d585fbd1ab6a/pkg/apis/runtime/v1alpha2/api.proto#L34-L128
