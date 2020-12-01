package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/release/pkg/util"
)

const (
	namespace = "cri-o-metrics-exporter"
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
		return errors.Wrap(err, "retrieving client config")
	}

	logrus.Info("Creating Kubernetes client")
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return errors.Wrap(err, "creating Kubernetes client")
	}

	logrus.Info("Retrieving nodes")
	ctx := context.Background()
	nodes, err := clientset.CoreV1().
		Nodes().
		List(ctx, metav1.ListOptions{})
	if err != nil {
		return errors.Wrap(err, "listing nodes")
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
			return errors.Wrap(err, "updating scrape config map")
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
		return errors.Wrap(err, "creating scrape config map")
	}
	logrus.Infof("Wrote scrape configs to configMap %s", configMap)

	addr := ":8080"
	logrus.Infof("Serving HTTP on %s", addr)
	return errors.Wrap(http.ListenAndServe(addr, nil), "running HTTP server")
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
		Host: fmt.Sprintf(
			"%s:%s", h.ip, util.EnvDefault("CRIO_METRICS_PORT", "9090"),
		),
		Path: "/metrics",
	}
	resp, err := http.Get(metricsEndpoint.String())
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

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logrus.Errorf("Unable to read metrics response: %v", err)
		return
	}

	if _, err := w.Write(bodyBytes); err != nil {
		logrus.Errorf("Unable to write response: %v", err)
	}
}
