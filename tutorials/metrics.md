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

## Exporting Metrics via Prometheus

The CRI-O metrics exporter can be used to provide a cluster wide scraping
endpoint for Prometheus. It is possible to either build the container image
manually via `make metrics-exporter` or directly consume the [available image on
quay.io][4].

[4]: https://quay.io/repository/crio/metrics-exporter

The deployment requires enabled [RBAC][5] within the target Kubernetes
environment and creates a new [ClusterRole][6] to be able to list available
nodes. Beside that a new Role wille be created to be able to update a config-map
within the `cri-o-exporter` namespace. Please be aware that the exporter only
works if the pod has access to the node IP from its namespace. This should
generally work but might be restricted due to network configuration or policies.

[5]: https://kubernetes.io/docs/reference/access-authn-authz/rbac
[6]: https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#clusterrole-v1-rbac-authorization-k8s-io

To deploy the metrics exporter within a new `cri-o-metrics-exporter` namespace,
simply apply the [cluster.yaml][7] from the root directory of this repository:

[7]: ../contrib/metrics-exporter/cluster.yaml

```
> kubectl create -f contrib/metrics-exporter/cluster.yaml
```

The `CRIO_METRICS_PORT` environment variable is set per default to `"9090"` and
can be used to customize the metrics port for the nodes. If the deployment is
up and running, it should log the registered nodes as well as that a new
config-map has been created:

```
> kubectl logs -f cri-o-metrics-exporter-65c9b7b867-7qmsb
level=info msg="Getting cluster configuration"
level=info msg="Creating Kubernetes client"
level=info msg="Retrieving nodes"
level=info msg="Registering handler /master (for 172.1.2.0)"
level=info msg="Registering handler /node-0 (for 172.1.3.0)"
level=info msg="Registering handler /node-1 (for 172.1.3.1)"
level=info msg="Registering handler /node-2 (for 172.1.3.2)"
level=info msg="Registering handler /node-3 (for 172.1.3.3)"
level=info msg="Registering handler /node-4 (for 172.1.3.4)"
level=info msg="Updated scrape configs in configMap cri-o-metrics-exporter"
level=info msg="Wrote scrape configs to configMap cri-o-metrics-exporter"
level=info msg="Serving HTTP on :8080"
```

The config-map now contains the [scrape configuration][8], which can be used for
Prometheus:

[8]: https://prometheus.io/docs/prometheus/latest/configuration/configuration/#scrape_config

```
> kubectl get cm cri-o-metrics-exporter -o yaml
```

```yaml
apiVersion: v1
data:
  config: |
    scrape_configs:
    - job_name: "cri-o-exporter-master"
      scrape_interval: 1s
      metrics_path: /master
      static_configs:
        - targets: ["cri-o-metrics-exporter.cri-o-metrics-exporter"]
          labels:
            instance: "master"
    - job_name: "cri-o-exporter-node-0"
      scrape_interval: 1s
      metrics_path: /node-0
      static_configs:
        - targets: ["cri-o-metrics-exporter.cri-o-metrics-exporter"]
          labels:
            instance: "node-0"
    - job_name: "cri-o-exporter-node-1"
      scrape_interval: 1s
      metrics_path: /node-1
      static_configs:
        - targets: ["cri-o-metrics-exporter.cri-o-metrics-exporter"]
          labels:
            instance: "node-1"
    - job_name: "cri-o-exporter-node-2"
      scrape_interval: 1s
      metrics_path: /node-2
      static_configs:
        - targets: ["cri-o-metrics-exporter.cri-o-metrics-exporter"]
          labels:
            instance: "node-2"
    - job_name: "cri-o-exporter-node-3"
      scrape_interval: 1s
      metrics_path: /node-3
      static_configs:
        - targets: ["cri-o-metrics-exporter.cri-o-metrics-exporter"]
          labels:
            instance: "node-3"
    - job_name: "cri-o-exporter-node-4"
      scrape_interval: 1s
      metrics_path: /node-4
      static_configs:
        - targets: ["cri-o-metrics-exporter.cri-o-metrics-exporter"]
          labels:
            instance: "node-4"
kind: ConfigMap
metadata:
  creationTimestamp: "2020-05-12T08:29:06Z"
  name: cri-o-metrics-exporter
  namespace: cri-o-metrics-exporter
  resourceVersion: "2862950"
  selfLink: /api/v1/namespaces/cri-o-metrics-exporter/configmaps/cri-o-metrics-exporter
  uid: 1409804a-78a2-4961-8205-c5f383626b4b
```

If the scrape configuration has been added to the Prometheus server, then the
provided [Grafana][9] dashboard [within this repository][10] can be setup, too:

[9]: https://grafana.com
[10]: ../contrib/metrics-exporter/dashboard.json

![grafana-setup](../contrib/metrics-exporter/dashboard.gif)
