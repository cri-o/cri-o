# Capture `cri-o` traces using OpenTelemetry

## Install OpenTelemetry

```sh
oc create ns otel
oc project otel
oc create sa otel && oc adm policy add-role-to-user admin -z otel && oc adm policy add-cluster-role-to-user cluster-admin -z otel
```

Create otel-agent and oytel-collector YAML objects from stdin

```sh
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: otel-agent-conf
  labels:
    app: opentelemetry
    component: otel-agent-conf
data:
  otel-agent-config: |
    receivers:
      otlp:
        protocols:
          grpc:
            max_recv_msg_size_mib: 999999999
          http:
    exporters:
      logging:
      otlp:
        endpoint: "ClusterIP:4317" # replace with the ClusterIP for otel-collector service
        insecure: true
    processors:
      batch:
    extensions:
      health_check: {}
    service:
      extensions: [health_check]
      pipelines:
        traces:
          receivers: [otlp]
          processors: [batch]
          exporters: [logging, otlp]
---
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: otel-agent
  labels:
    app: opentelemetry
    component: otel-agent
spec:
  selector:
    matchLabels:
      app: opentelemetry
      component: otel-agent
  template:
    metadata:
      labels:
        app: opentelemetry
        component: otel-agent
    spec:
      securityContext: {}
      serviceAccount: otel
      serviceAccountName: otel
      hostNetwork: true
      containers:
      - command:
          - "/otelcol"
          - "--config=/conf/otel-agent-config.yaml"
          # Memory Ballast size should be max 1/3 to 1/2 of memory.
          - "--mem-ballast-size-mib=165"
        image: otel/opentelemetry-collector-dev:latest
        name: otel-agent
        resources:
          limits:
            cpu: "1"
            memory: 1Gi
          requests:
            cpu: 500m
            memory: 500Mi
        ports:
        - containerPort: 4317 # Default OpenTelemetry receiver port.
        - containerPort: 8888  # Metrics.
        volumeMounts:
        - name: otel-agent-config-vol
          mountPath: /conf
        livenessProbe:
          httpGet:
            path: /
            port: 13133 # Health Check extension default port.
        readinessProbe:
          httpGet:
            path: /
            port: 13133 # Health Check extension default port.
      volumes:
        - configMap:
            name: otel-agent-conf
            items:
              - key: otel-agent-config
                path: otel-agent-config.yaml
          name: otel-agent-config-vol
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: otel-collector-conf
  labels:
    app: opentelemetry
    component: otel-collector-conf
data:
  otel-collector-config: |
    receivers:
      otlp:
        protocols:
          grpc:
            max_recv_msg_size_mib: 999999999
    processors:
      batch:
    extensions:
      health_check: {}
    exporters:
      logging:
      zipkin:
        endpoint: "http://somezipkin.target.com:9411/api/v2/spans" # Replace with a real endpoint.
      jaeger:
        endpoint: "jaeger-collector.otel.svc.cluster.local:14250" # Replace with a real endpoint.
        insecure: true
    service:
      extensions: [health_check]
      pipelines:
        traces/1:
          receivers: [otlp]
          processors: [batch]
          exporters: [logging, jaeger]
---
apiVersion: v1
kind: Service
metadata:
  name: otel-collector
  labels:
    app: opentelemetry
    component: otel-collector
spec:
  ports:
  - name: otlp # Default endpoint for OpenTelemetry receiver.
    port: 4317
    protocol: TCP
    targetPort: 4317
  - name: jaeger-https # Default endpoint for Jaeger gRPC receiver
    port: 14250
    targetPort: 14250
  - name: jaeger-thrift-http # Default endpoint for Jaeger HTTP receiver.
    port: 14268
  - name: zipkin # Default endpoint for Zipkin receiver.
    port: 9411
  - name: metrics # Default endpoint for querying metrics.
    port: 8888
  selector:
    component: otel-collector
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: otel-collector
  labels:
    app: opentelemetry
    component: otel-collector
spec:
  selector:
    matchLabels:
      app: opentelemetry
      component: otel-collector
  minReadySeconds: 5
  progressDeadlineSeconds: 120
  replicas: 1 #TODO - adjust this to your own requirements
  template:
    metadata:
      labels:
        app: opentelemetry
        component: otel-collector
    spec:
      containers:
      - command:
          - "/otelcol"
          - "--config=/conf/otel-collector-config.yaml"
#           Memory Ballast size should be max 1/3 to 1/2 of memory.
          - "--mem-ballast-size-mib=683"
        image: otel/opentelemetry-collector-dev:latest
        name: otel-collector
        resources:
          limits:
            cpu: 1
            memory: 2Gi
          requests:
            cpu: 500m
            memory: 1Gi
        ports:
        - containerPort: 55679 # Default endpoint for ZPages.
        - containerPort: 4317 # Default endpoint for OpenTelemetry receiver.
        - containerPort: 14250 # Default endpoint for Jaeger HTTP receiver.
        - containerPort: 14268 # Default endpoint for Jaeger HTTP receiver.
        - containerPort: 9411 # Default endpoint for Zipkin receiver.
        - containerPort: 8888  # Default endpoint for querying metrics.
        env:
        - name: JAEGER_AGENT_HOST
          valueFrom:
            fieldRef:
              fieldPath: status.hostIP
        volumeMounts:
        - name: otel-collector-config-vol
          mountPath: /conf
        livenessProbe:
          httpGet:
            path: /
            port: 13133 # Health Check extension default port.
        readinessProbe:
          httpGet:
            path: /
            port: 13133 # Health Check extension default port.
      volumes:
        - configMap:
            name: otel-collector-conf
            items:
              - key: otel-collector-config
                path: otel-collector-config.yaml
          name: otel-collector-config-vol
EOF
```
This will create 2 configmaps `` and ``. `otel-agent` is created as a `DaemonSet` and `otel-collector` is created as a `Deployment`

```
watch oc get configmaps, ds, deployment, services
```
Once the service is up and running take the `ClusterIp` and update the otlp exporter endpoint in the configmap as

```
 oc edit cm/otel-agent-conf -o yaml

 exporters:
      logging:
      otlp:
        endpoint: "ClusterIP:4317" # replace with the ClusterIP for otel-collector service

```
Now delete the three agent pods so that the otel-agent DaemonSet can launch new pods with updated endpoint. 

`oc delete pods --selector=component=otel-agent`

Check otel-collector pod logs to see traces. You should see traces like
```
2021-06-15T13:38:56.990Z        INFO    loggingexporter/logging_exporter.go:42  TracesExporter  {"#spans": 110}
2021-06-15T13:38:58.995Z        INFO    loggingexporter/logging_exporter.go:42  TracesExporter  {"#spans": 23}
2021-06-15T13:39:02.001Z        INFO    loggingexporter/logging_exporter.go:42  TracesExporter  {"#spans": 55}
2021-06-15T13:39:04.005Z        INFO    loggingexporter/logging_exporter.go:42  TracesExporter  {"#spans": 77}
```

Since `Jaeger` is not running right now you will also notice below error in the collector log but that will be resolved as soon as you install and create Jaeger. 
```
Exporting failed. Will retry the request after interval.        {"kind": "exporter", "name": "jaeger", "error": "failed to push trace data via Jaeger exporter: rpc error: code = Unavailable desc = connection error: desc = \"transport: Error while dialing dial tcp: lookup jaeger-collector.otel.svc.cluster.local on 172.30.0.10:53: no such host\"", "interval": "5.934115365s"}
```

## Steps to install Jaeger
