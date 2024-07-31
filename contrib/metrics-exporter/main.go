package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/release-utils/env"
)

var (
	namespace = env.Default("POD_NAMESPACE", "cri-o-metrics-exporter")
	service   = namespace
	configMap = namespace
)

func main() {
	if err := run(); err != nil {
		logrus.Fatalf("Unable to run: %v", err)
	}
}

func run() error {
	logrus.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true})

	logrus.Info("Getting cluster configuration")
	config, err := rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("retrieving client config: %w", err)
	}

	logrus.Info("Creating Kubernetes client")
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("creating Kubernetes client: %w", err)
	}

	logrus.Info("Retrieving nodes")
	ctx := context.Background()
	nodes, err := clientset.CoreV1().
		Nodes().
		List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing nodes: %w", err)
	}

	jobConfigs := []string{}
	for i := range nodes.Items {
		node := nodes.Items[i]
		nodeAddress := node.Status.Addresses[0].Address

		logrus.Infof("Registering handler /%s (for %s)", node.Name, nodeAddress)
		http.Handle("/"+node.Name, &handler{nodeAddress})
		jobConfigs = append(jobConfigs, jobConfig(node.Name))
	}

	scrapeConfigs := fmt.Sprintf(
		"scrape_configs:\n%s\n", strings.Join(jobConfigs, "\n"),
	)

	cm, err := clientset.CoreV1().
		ConfigMaps(namespace).
		Get(ctx, configMap, metav1.GetOptions{})
	const key = "config"
	if err == nil {
		cm.Data[key] = scrapeConfigs
		if _, err := clientset.CoreV1().
			ConfigMaps(namespace).
			Update(ctx, cm, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("updating scrape config map: %w", err)
		}
		logrus.Infof("Updated scrape configs in configMap %s", configMap)
	} else if _, err := clientset.CoreV1().
		ConfigMaps(namespace).
		Create(ctx, &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMap,
				Namespace: namespace,
			},
			Data: map[string]string{
				"config": scrapeConfigs,
			},
		}, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("creating scrape config map: %w", err)
	}
	logrus.Infof("Wrote scrape configs to configMap %s", configMap)

	addr := ":8080"
	logrus.Infof("Serving HTTP on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		return fmt.Errorf("running HTTP server: %w", err)
	}

	return nil
}

func jobConfig(name string) string {
	return fmt.Sprintf(`- job_name: "cri-o-exporter-%s"
  scrape_interval: 1s
  metrics_path: /%s
  static_configs:
    - targets: ["%s.%s"]
      labels:
        instance: %q`, name, name, service, namespace, name)
}

type handler struct {
	ip string
}

func (h *handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	metricsEndpoint := url.URL{
		Scheme: "http",
		Host: net.JoinHostPort(
			h.ip, env.Default("CRIO_METRICS_PORT", "9090"),
		),
		Path: "/metrics",
	}
	metricsReq, err := http.NewRequestWithContext(req.Context(), http.MethodGet, metricsEndpoint.String(), http.NoBody)
	if err != nil {
		logrus.Errorf(
			"Unable to create metrics request %s: %v",
			metricsEndpoint.String(), err,
		)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	resp, err := http.DefaultClient.Do(metricsReq)
	if err != nil {
		logrus.Errorf(
			"Unable to retrieve metrics from %s: %v",
			metricsEndpoint.String(), err,
		)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		w.WriteHeader(resp.StatusCode)
		return
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		logrus.Errorf("Unable to read metrics response: %v", err)
		return
	}

	if _, err := w.Write(bodyBytes); err != nil {
		logrus.Errorf("Unable to write response: %v", err)
	}
}
