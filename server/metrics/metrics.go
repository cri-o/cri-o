package metrics

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/cri-o/cri-o/internal/process"
	"github.com/cri-o/cri-o/internal/storage/references"
	libconfig "github.com/cri-o/cri-o/pkg/config"
	"github.com/cri-o/cri-o/server/otel-collector/collectors"
	"github.com/fsnotify/fsnotify"
	"github.com/opencontainers/go-digest"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/util/cert"
)

// SinceInMicroseconds gets the time since the specified start in microseconds.
func SinceInMicroseconds(start time.Time) float64 {
	return float64(time.Since(start).Microseconds())
}

// SinceInSeconds gets the time since specified start in seconds.
func SinceInSeconds(start time.Time) float64 {
	return time.Since(start).Seconds()
}

// GetSizeBucket gets a bucket name for given size sorted according to 1 KiB, 1 MiB, 10 MiB, ...
func GetSizeBucket(size float64) string {
	sizeBuckets := []float64{
		10 * 1000 * 1000 * 1000, //  10 GiB
		1000 * 1000 * 1000,      //   1 GiB
		500 * 1000 * 1000,       // 500 MiB
		400 * 1000 * 1000,       // 400 MiB
		300 * 1000 * 1000,       // 300 MiB
		200 * 1000 * 1000,       // 200 MiB
		100 * 1000 * 1000,       // 100 MiB
		50 * 1000 * 1000,        //  50 MiB
		10 * 1000 * 1000,        //  10 MiB
		1000 * 1000,             //   1 MiB
		1000,                    //   1 KiB
	}
	sizeBucketNames := []string{
		">10 GiB", ">1 GiB", ">500 MiB", ">400 MiB",
		">300 MiB", ">200 MiB", ">100 MiB", ">50 MiB",
		">10 MiB", ">1 MiB", ">1 KiB",
	}
	for bucketIdx := range sizeBuckets {
		if size > sizeBuckets[bucketIdx] {
			return sizeBucketNames[bucketIdx]
		}
	}
	return ">0 B"
}

// Metrics is the main structure for starting the metrics endpoints.
type Metrics struct {
	config                                    *libconfig.MetricsConfig
	metricOperations                          *prometheus.CounterVec // Deprecated: in favour of metricOperationsTotal
	metricOperationsLatency                   *prometheus.GaugeVec   // Deprecated: in favour of metricOperationsLatencySeconds
	metricOperationsLatencyTotal              *prometheus.SummaryVec // Deprecated: in favour of metricOperationsLatencySecondsTotal
	metricOperationsErrors                    *prometheus.CounterVec // Deprecated: in favour of metricOperationsErrorsTotal
	metricImagePullsByDigest                  *prometheus.CounterVec // Deprecated: in favour of metricImagePullsBytesTotal
	metricImagePullsByName                    *prometheus.CounterVec // Deprecated: in favour of metricImagePullsBytesTotal
	metricImagePullsByNameSkipped             *prometheus.CounterVec // Deprecated: in favour of metricImagePullsSkippedBytesTotal
	metricImagePullsFailures                  *prometheus.CounterVec // Deprecated: in favour of metricImagePullsFailureTotal
	metricImagePullsSuccesses                 *prometheus.CounterVec // Deprecated: in favour of metricImagePullsSuccessTotal
	metricImagePullsLayerSize                 prometheus.Histogram
	metricImageLayerReuse                     *prometheus.CounterVec // Deprecated: in favour of metricImageLayerReuseTotal
	metricContainersEventsDropped             prometheus.Counter
	metricContainersOOMTotal                  prometheus.Counter
	metricContainersOOM                       *prometheus.CounterVec // Deprecated: in favour of metricContainersOOMCountTotal
	metricProcessesDefunct                    prometheus.GaugeFunc
	metricOperationsTotal                     *prometheus.CounterVec
	metricOperationsLatencySeconds            *prometheus.GaugeVec
	metricOperationsLatencySecondsTotal       *prometheus.SummaryVec
	metricOperationsErrorsTotal               *prometheus.CounterVec
	metricImagePullsBytesTotal                *prometheus.CounterVec
	metricImagePullsSkippedBytesTotal         *prometheus.CounterVec
	metricImagePullsFailureTotal              *prometheus.CounterVec
	metricImagePullsSuccessTotal              prometheus.Counter
	metricImageLayerReuseTotal                *prometheus.CounterVec
	metricContainersOOMCountTotal             *prometheus.CounterVec
	metricContainersSeccompNotifierCountTotal *prometheus.CounterVec
	metricResourcesStalledAtStage             *prometheus.CounterVec
}

var instance *Metrics

// New creates a new metrics instance.
func New(config *libconfig.MetricsConfig) *Metrics {
	instance = &Metrics{
		config: config,
		metricOperations: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: collectors.Subsystem,
				Name:      collectors.Operations.String(),
				Help:      "[DEPRECATED: in favour of `operations_total`] Cumulative number of CRI-O operations by operation type.",
			},
			[]string{"operation_type"},
		),
		metricOperationsLatency: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Subsystem: collectors.Subsystem,
				Name:      collectors.OperationsLatency.String(),
				Help:      "[DEPRECATED: in favour of `operations_latency_seconds`] Latency in microseconds of individual CRI calls for CRI-O operations. Broken down by operation type.",
			},
			[]string{"operation_type"},
		),
		metricOperationsLatencyTotal: prometheus.NewSummaryVec(
			prometheus.SummaryOpts{
				Subsystem:  collectors.Subsystem,
				Name:       collectors.OperationsLatencyTotal.String(),
				Help:       "[DEPRECATED:  in favour of `operations_latency_seconds_total`] Latency in microseconds of CRI-O operations. Broken down by operation type.",
				Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
			},
			[]string{"operation_type"},
		),
		metricOperationsErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: collectors.Subsystem,
				Name:      collectors.OperationsErrors.String(),
				Help:      "[DEPRECATED: in favour of `operations_errors_total`] Cumulative number of CRI-O operation errors by operation type.",
			},
			[]string{"operation_type"},
		),
		metricImagePullsByDigest: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: collectors.Subsystem,
				Name:      collectors.ImagePullsByDigest.String(),
				Help:      "[DEPRECATED: in favour of `image_pulls_bytes_total`] Bytes transferred by CRI-O image pulls by digest",
			},
			[]string{"name", "digest", "mediatype", "size"},
		),
		metricImagePullsByName: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: collectors.Subsystem,
				Name:      collectors.ImagePullsByName.String(),
				Help:      "[DEPRECATED: in favour of `image_pulls_bytes_total`] Bytes transferred by CRI-O image pulls by name",
			},
			[]string{"name", "size"},
		),
		metricImagePullsByNameSkipped: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: collectors.Subsystem,
				Name:      collectors.ImagePullsByNameSkipped.String(),
				Help:      "[DEPRECATED: in favour of `image_pulls_skipped_bytes_total`] Bytes skipped by CRI-O image pulls by name",
			},
			[]string{"name"},
		),
		metricImagePullsFailures: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: collectors.Subsystem,
				Name:      collectors.ImagePullsFailures.String(),
				Help:      "[DEPRECATED: in favour of `image_pulls_failure_total`] Cumulative number of CRI-O image pull failures by error.",
			},
			[]string{"name", "error"},
		),
		metricImagePullsSuccesses: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: collectors.Subsystem,
				Name:      collectors.ImagePullsSuccesses.String(),
				Help:      "[DEPRECATED: in favour of `image_pulls_success_total`] Cumulative number of CRI-O image pull successes.",
			},
			[]string{"name"},
		),
		metricImagePullsLayerSize: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Subsystem: collectors.Subsystem,
				Name:      collectors.ImagePullsLayerSize.String(),
				Help:      "Bytes transferred by CRI-O image pulls per layer",
				Buckets: []float64{ // in bytes
					1000,                    //   1 KiB
					1000 * 1000,             //   1 MiB
					10 * 1000 * 1000,        //  10 MiB
					50 * 1000 * 1000,        //  50 MiB
					100 * 1000 * 1000,       // 100 MiB
					200 * 1000 * 1000,       // 200 MiB
					300 * 1000 * 1000,       // 300 MiB
					400 * 1000 * 1000,       // 400 MiB
					500 * 1000 * 1000,       // 500 MiB
					1000 * 1000 * 1000,      //   1 GiB
					10 * 1000 * 1000 * 1000, //  10 GiB
				},
			},
		),
		metricImageLayerReuse: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: collectors.Subsystem,
				Name:      collectors.ImageLayerReuse.String(),
				Help:      "[DEPRECATED: in favour of `image_layer_reuse_total`] Reused (not pulled) local image layer count by name",
			},
			[]string{"name"},
		),
		metricContainersEventsDropped: prometheus.NewCounter(
			prometheus.CounterOpts{
				Subsystem: collectors.Subsystem,
				Name:      collectors.ContainersEventsDropped.String(),
				Help:      "Amount of container events dropped",
			},
		),
		metricContainersOOMTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Subsystem: collectors.Subsystem,
				Name:      collectors.ContainersOOMTotal.String(),
				Help:      "Amount of containers killed because they ran out of memory (OOM)",
			},
		),
		metricContainersOOM: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: collectors.Subsystem,
				Name:      collectors.ContainersOOM.String(),
				Help:      "[DEPRECATED: in favour of `containers_oom_count_total`] Amount of containers killed because they ran out of memory (OOM) by their name",
			},
			[]string{"name"},
		),
		metricProcessesDefunct: prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{
				Subsystem: collectors.Subsystem,
				Name:      collectors.ProcessesDefunct.String(),
				Help:      "Total number of defunct processes in the node",
			},
			func() float64 {
				total, err := process.DefunctProcesses()
				if err == nil {
					return float64(total)
				}
				logrus.Warn(err)
				return 0
			},
		),
		metricOperationsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: collectors.Subsystem,
				Name:      collectors.OperationsTotal.String(),
				Help:      "Cumulative number of CRI-O operations by operation type.",
			},
			[]string{"operation"},
		),
		metricOperationsLatencySeconds: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Subsystem: collectors.Subsystem,
				Name:      collectors.OperationsLatencySeconds.String(),
				Help:      "Latency in seconds of individual CRI calls for CRI-O operations. Broken down by operation type.",
			},
			[]string{"operation"},
		),
		metricOperationsLatencySecondsTotal: prometheus.NewSummaryVec(
			prometheus.SummaryOpts{
				Subsystem:  collectors.Subsystem,
				Name:       collectors.OperationsLatencySecondsTotal.String(),
				Help:       "Latency in seconds of CRI-O operations. Broken down by operation type.",
				Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
			},
			[]string{"operation"},
		),
		metricOperationsErrorsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: collectors.Subsystem,
				Name:      collectors.OperationsErrorsTotal.String(),
				Help:      "Cumulative number of CRI-O operation errors by operation type.",
			},
			[]string{"operation"},
		),
		metricImagePullsBytesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: collectors.Subsystem,
				Name:      collectors.ImagePullsBytesTotal.String(),
				Help:      "Bytes transferred by CRI-O image pulls",
			},
			[]string{"mediatype", "size"},
		),
		metricImagePullsSkippedBytesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: collectors.Subsystem,
				Name:      collectors.ImagePullsSkippedBytesTotal.String(),
				Help:      "Bytes skipped by CRI-O image pulls",
			},
			[]string{"size"},
		),
		metricImagePullsFailureTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: collectors.Subsystem,
				Name:      collectors.ImagePullsFailureTotal.String(),
				Help:      "Cumulative number of CRI-O image pull failures by error.",
			},
			[]string{"error"},
		),
		metricImagePullsSuccessTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Subsystem: collectors.Subsystem,
				Name:      collectors.ImagePullsSuccessTotal.String(),
				Help:      "Cumulative number of CRI-O image pull successes.",
			},
		),
		metricImageLayerReuseTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: collectors.Subsystem,
				Name:      collectors.ImageLayerReuseTotal.String(),
				Help:      "Reused (not pulled) local image layer count by name",
			},
			[]string{"name"},
		),
		metricContainersOOMCountTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: collectors.Subsystem,
				Name:      collectors.ContainersOOMCountTotal.String(),
				Help:      "Amount of containers killed because they ran out of memory (OOM) by their name",
			},
			// The label `name` can have high cardinality sometimes but it is in the interest of users giving them the
			// ease to identify which container(s) are going into OOM state. Also, ideally very few containers should OOM
			// keeping the label cardinality of `name` reasonably low.
			[]string{"name"},
		),
		metricContainersSeccompNotifierCountTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: collectors.Subsystem,
				Name:      collectors.ContainersSeccompNotifierCountTotal.String(),
				Help:      "Number of forbidden syscalls by syscall and container name",
			},
			[]string{"name", "syscall"},
		),
		metricResourcesStalledAtStage: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: collectors.Subsystem,
				Name:      collectors.ResourcesStalledAtStage.String(),
				Help:      "Resource creation stage pod or container is stalled at.",
			},
			[]string{"stage"},
		),
	}
	return Instance()
}

// Instance returns the singleton instance of the Metrics.
func Instance() *Metrics {
	if instance == nil {
		return New(&libconfig.MetricsConfig{})
	}
	return instance
}

// Start starts serving the metrics in the background.
func (m *Metrics) Start(stop chan struct{}) error {
	if m.config == nil {
		return errors.New("provided config is nil")
	}

	me, err := m.createEndpoint()
	if err != nil {
		return fmt.Errorf("create endpoint: %w", err)
	}

	if err := m.startEndpoint(
		stop, "tcp", net.JoinHostPort(m.config.MetricsHost, strconv.Itoa(m.config.MetricsPort)), me,
	); err != nil {
		return fmt.Errorf(
			"create metrics endpoint on port %d: %w", m.config.MetricsPort, err,
		)
	}

	metricsSocket := m.config.MetricsSocket
	if metricsSocket != "" {
		if err := libconfig.RemoveUnusedSocket(metricsSocket); err != nil {
			return fmt.Errorf("removing unused socket %s: %w", metricsSocket, err)
		}

		if err := m.startEndpoint(stop, "unix", m.config.MetricsSocket, me); err != nil {
			return fmt.Errorf("creating metrics endpoint socket: %w", err)
		}
		return nil
	}

	return nil
}

func (m *Metrics) MetricOperationsInc(operation string) {
	c, err := m.metricOperations.GetMetricWithLabelValues(operation) // deprecated metric name
	if err != nil {
		logrus.Warnf("Unable to write operations metric: %v", err)
		return
	}
	c.Inc()

	c, err = m.metricOperationsTotal.GetMetricWithLabelValues(operation)
	if err != nil {
		logrus.Warnf("Unable to write operations metric: %v", err)
		return
	}
	c.Inc()
}

func (m *Metrics) MetricOperationsLatencySet(operation string, start time.Time) {
	g, err := m.metricOperationsLatency.GetMetricWithLabelValues(operation) // deprecated metric name
	if err != nil {
		logrus.Warnf("Unable to write operation latency metric: %v", err)
		return
	}
	g.Set(SinceInMicroseconds(start))

	g, err = m.metricOperationsLatencySeconds.GetMetricWithLabelValues(operation)
	if err != nil {
		logrus.Warnf("Unable to write operation latency metric: %v", err)
		return
	}
	g.Set(SinceInSeconds(start))
}

func (m *Metrics) MetricOperationsLatencyTotalObserve(operation string, start time.Time) {
	o, err := m.metricOperationsLatencyTotal.GetMetricWithLabelValues(operation) // deprecated metric name
	if err != nil {
		logrus.Warnf("Unable to write operation latency (total) metric: %v", err)
		return
	}
	o.Observe(SinceInMicroseconds(start))

	o, err = m.metricOperationsLatencySecondsTotal.GetMetricWithLabelValues(operation)
	if err != nil {
		logrus.Warnf("Unable to write operation latency (total) metric: %v", err)
		return
	}
	o.Observe(SinceInSeconds(start))
}

func (m *Metrics) MetricOperationsErrorsInc(operation string) {
	c, err := m.metricOperationsErrors.GetMetricWithLabelValues(operation) // deprecated metric name
	if err != nil {
		logrus.Warnf("Unable to write operation errors metric: %v", err)
		return
	}
	c.Inc()

	c, err = m.metricOperationsErrorsTotal.GetMetricWithLabelValues(operation)
	if err != nil {
		logrus.Warnf("Unable to write operation errors metric: %v", err)
		return
	}
	c.Inc()
}

func (m *Metrics) MetricContainersOOMCountTotalInc(name string) {
	c, err := m.metricContainersOOM.GetMetricWithLabelValues(name) // deprecated metric name
	if err != nil {
		logrus.Warnf("Unable to write container OOM metric: %v", err)
		return
	}
	c.Inc()

	c, err = m.metricContainersOOMCountTotal.GetMetricWithLabelValues(name)
	if err != nil {
		logrus.Warnf("Unable to write container OOM metric: %v", err)
		return
	}
	c.Inc()
}

func (m *Metrics) MetricContainersOOMCountTotalDelete(name string) {
	m.metricContainersOOM.DeleteLabelValues(name)
	m.metricContainersOOMCountTotal.DeleteLabelValues(name)
}

func (m *Metrics) MetricContainersEventsDroppedInc() {
	m.metricContainersEventsDropped.Inc()
}

func (m *Metrics) MetricContainersOOMTotalInc() {
	m.metricContainersOOMTotal.Inc()
}

func (m *Metrics) MetricContainersSeccompNotifierCountTotalInc(name, syscall string) {
	c, err := m.metricContainersSeccompNotifierCountTotal.GetMetricWithLabelValues(name, syscall)
	if err != nil {
		logrus.Warnf("Unable to write container seccomp notifier metric: %v", err)
		return
	}
	c.Inc()
}

func (m *Metrics) MetricImagePullsLayerSizeObserve(size int64) {
	m.metricImagePullsLayerSize.Observe(float64(size))
}

func (m *Metrics) MetricImagePullsByNameSkippedAdd(add float64, name references.RegistryImageReference) {
	c, err := m.metricImagePullsByNameSkipped.GetMetricWithLabelValues(name.StringForOutOfProcessConsumptionOnly()) // deprecated metric name
	if err != nil {
		logrus.Warnf("Unable to write image pulls by name skipped metric: %v", err)
		return
	}
	c.Add(add)
}

func (m *Metrics) MetricImagePullsSkippedBytesAdd(add float64) {
	c, err := m.metricImagePullsSkippedBytesTotal.GetMetricWithLabelValues(GetSizeBucket(add))
	if err != nil {
		logrus.Warnf("Unable to write image pulls skipped bytes metric: %v", err)
		return
	}
	c.Add(add)
}

func (m *Metrics) MetricImagePullsFailuresInc(image references.RegistryImageReference, label string) {
	c, err := m.metricImagePullsFailures.GetMetricWithLabelValues(image.StringForOutOfProcessConsumptionOnly(), label) // deprecated metric name
	if err != nil {
		logrus.Warnf("Unable to write image pull failures metric: %v", err)
		return
	}
	c.Inc()

	c, err = m.metricImagePullsFailureTotal.GetMetricWithLabelValues(label)
	if err != nil {
		logrus.Warnf("Unable to write image pull failures total metric: %v", err)
		return
	}
	c.Inc()
}

func (m *Metrics) MetricImageLayerReuseInc(layer string) {
	c, err := m.metricImageLayerReuse.GetMetricWithLabelValues(layer) // deprecated metric name
	if err != nil {
		logrus.Warnf("Unable to write image layer reuse metric: %v", err)
		return
	}
	c.Inc()

	c, err = m.metricImageLayerReuseTotal.GetMetricWithLabelValues(layer)
	if err != nil {
		logrus.Warnf("Unable to write image layer reuse total metric: %v", err)
		return
	}
	c.Inc()
}

func (m *Metrics) MetricImagePullsSuccessesInc(name references.RegistryImageReference) {
	c, err := m.metricImagePullsSuccesses.GetMetricWithLabelValues(name.StringForOutOfProcessConsumptionOnly()) // deprecated metric name
	if err != nil {
		logrus.Warnf("Unable to write image pull successes metric: %v", err)
		return
	}
	c.Inc()

	m.metricImagePullsSuccessTotal.Inc()
}

func (m *Metrics) MetricImagePullsBytesAdd(add float64, mediatype string, size int64) {
	c, err := m.metricImagePullsBytesTotal.GetMetricWithLabelValues(mediatype, GetSizeBucket(float64(size)))
	if err != nil {
		logrus.Warnf("Unable to write image pulls bytes metric: %v", err)
		return
	}
	c.Add(add)
}

func (m *Metrics) MetricImagePullsByDigestAdd(add float64, name references.RegistryImageReference, artifact digest.Digest, values ...string) {
	c, err := m.metricImagePullsByDigest.GetMetricWithLabelValues(append([]string{name.StringForOutOfProcessConsumptionOnly(), artifact.String()}, values...)...) // deprecated metric name
	if err != nil {
		logrus.Warnf("Unable to write image pulls by digest metric: %v", err)
		return
	}
	c.Add(add)
}

func (m *Metrics) MetricImagePullsByNameAdd(add float64, name references.RegistryImageReference, values ...string) {
	c, err := m.metricImagePullsByName.GetMetricWithLabelValues(append([]string{name.StringForOutOfProcessConsumptionOnly()}, values...)...) // deprecated metric name
	if err != nil {
		logrus.Warnf("Unable to write image pulls by name metric: %v", err)
		return
	}
	c.Add(add)
}

func (m *Metrics) MetricResourcesStalledAtStage(stage string) {
	c, err := m.metricResourcesStalledAtStage.GetMetricWithLabelValues(stage)
	if err != nil {
		logrus.Warnf("Unable to write resource stalled at stage metric: %v", err)
		return
	}
	c.Inc()
}

// createEndpoint creates a /metrics endpoint for prometheus monitoring.
func (m *Metrics) createEndpoint() (*http.ServeMux, error) {
	for collector, metric := range map[collectors.Collector]prometheus.Collector{
		collectors.Operations:              m.metricOperations,
		collectors.OperationsLatency:       m.metricOperationsLatency,
		collectors.OperationsLatencyTotal:  m.metricOperationsLatencyTotal,
		collectors.OperationsErrors:        m.metricOperationsErrors,
		collectors.ImagePullsByDigest:      m.metricImagePullsByDigest,
		collectors.ImagePullsByName:        m.metricImagePullsByName,
		collectors.ImagePullsByNameSkipped: m.metricImagePullsByNameSkipped,
		collectors.ImagePullsFailures:      m.metricImagePullsFailures,
		collectors.ImagePullsSuccesses:     m.metricImagePullsSuccesses,
		collectors.ImagePullsLayerSize:     m.metricImagePullsLayerSize,
		collectors.ImageLayerReuse:         m.metricImageLayerReuse,
		collectors.ContainersEventsDropped: m.metricContainersEventsDropped,
		collectors.ContainersOOMTotal:      m.metricContainersOOMTotal,
		collectors.ContainersOOM:           m.metricContainersOOM,
		collectors.ProcessesDefunct:        m.metricProcessesDefunct,

		collectors.OperationsTotal:                     m.metricOperationsTotal,
		collectors.OperationsLatencySeconds:            m.metricOperationsLatencySeconds,
		collectors.OperationsLatencySecondsTotal:       m.metricOperationsLatencySecondsTotal,
		collectors.OperationsErrorsTotal:               m.metricOperationsErrorsTotal,
		collectors.ImagePullsBytesTotal:                m.metricImagePullsBytesTotal,
		collectors.ImagePullsSkippedBytesTotal:         m.metricImagePullsSkippedBytesTotal,
		collectors.ImagePullsFailureTotal:              m.metricImagePullsFailureTotal,
		collectors.ImagePullsSuccessTotal:              m.metricImagePullsSuccessTotal,
		collectors.ImageLayerReuseTotal:                m.metricImageLayerReuseTotal,
		collectors.ContainersOOMCountTotal:             m.metricContainersOOMCountTotal,
		collectors.ContainersSeccompNotifierCountTotal: m.metricContainersSeccompNotifierCountTotal,
		collectors.ResourcesStalledAtStage:             m.metricResourcesStalledAtStage,
	} {
		if m.config.MetricsCollectors.Contains(collector) {
			logrus.Debugf("Enabling metric: %s", collector.Stripped())
			if err := prometheus.Register(metric); err != nil {
				return nil, fmt.Errorf("register metric: %w", err)
			}
		} else {
			logrus.Debugf("Skipping metric: %s", collector.Stripped())
		}
	}

	mux := &http.ServeMux{}
	mux.Handle("/metrics", promhttp.Handler())
	return mux, nil
}

func (m *Metrics) startEndpoint(
	stop chan struct{}, network, address string, me http.Handler,
) error {
	l, err := net.Listen(network, address)
	if err != nil {
		return fmt.Errorf("creating listener: %w", err)
	}

	go func() {
		var err error

		srv := http.Server{
			Handler: me,
		}

		if m.config.MetricsCert != "" && m.config.MetricsKey != "" {
			logrus.Infof("Serving metrics on %s via HTTPs", address)

			kpr, reloadErr := newCertReloader(
				stop, m.config.MetricsCert, m.config.MetricsKey,
			)
			if reloadErr != nil {
				logrus.Fatalf("Creating key pair reloader: %v", reloadErr)
			}

			srv.TLSConfig = &tls.Config{
				GetCertificate: kpr.getCertificate,
				MinVersion:     tls.VersionTLS12,
			}

			go func() {
				<-stop
				if err := srv.Shutdown(context.Background()); err != nil {
					logrus.Errorf("Error on metrics server shutdown: %v", err)
				}
			}()
			err = srv.ServeTLS(l, m.config.MetricsCert, m.config.MetricsKey)
		} else {
			logrus.Infof("Serving metrics on %s via HTTP", address)
			go func() {
				<-stop
				if err := srv.Shutdown(context.Background()); err != nil {
					logrus.Errorf("Error on metrics server shutdown: %v", err)
				}
			}()
			err = srv.Serve(l)
		}

		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logrus.Errorf("Failed to serve metrics endpoint %v: %v", l, err)
		}
	}()

	return nil
}

type certReloader struct {
	certLock    sync.RWMutex
	certificate *tls.Certificate
	certPath    string
	keyPath     string
}

func newCertReloader(doneChan chan struct{}, certPath, keyPath string) (*certReloader, error) {
	reloader := &certReloader{
		certPath: certPath,
		keyPath:  keyPath,
	}

	// Generate self-signed certificate and key if the provided ones are not
	// available.
	_, errCertPath := os.Stat(certPath)
	_, errKeyPath := os.Stat(keyPath)
	if errCertPath != nil && os.IsNotExist(errCertPath) &&
		errKeyPath != nil && os.IsNotExist(errKeyPath) {
		logrus.Info("Metrics key and cert path does not exist, generating self-signed")

		hostname, err := os.Hostname()
		if err != nil {
			return nil, fmt.Errorf("retrieve hostname: %w", err)
		}

		certBytes, keyBytes, err := cert.GenerateSelfSignedCertKey(hostname, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("generate self-signed cert/key: %w", err)
		}

		for path, bytes := range map[string][]byte{
			certPath: certBytes,
			keyPath:  keyBytes,
		} {
			if err := os.MkdirAll(filepath.Dir(path), os.FileMode(0o700)); err != nil {
				return nil, fmt.Errorf("create path: %w", err)
			}
			if err := os.WriteFile(path, bytes, os.FileMode(0o600)); err != nil {
				return nil, fmt.Errorf("write file: %w", err)
			}
		}
	}

	if err := reloader.reload(); err != nil {
		return nil, fmt.Errorf("load certificate: %w", err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create new watcher: %w", err)
	}
	go func() {
		defer watcher.Close()
		done := make(chan struct{})
		go func() {
			for {
				select {
				case event := <-watcher.Events:
					logrus.Debugf(
						"Got cert watcher event for %s (%s), reloading certificates",
						event.Name, event.Op.String(),
					)
					if err := reloader.reload(); err != nil {
						logrus.Warnf("Keeping previous certificates: %v", err)
					}
				case err := <-watcher.Errors:
					logrus.Errorf("Cert watcher error: %v", err)
					close(done)
					return
				case <-doneChan:
					logrus.Debug("Closing cert watcher")
					close(done)
					return
				}
			}
		}()
		for _, f := range []string{certPath, keyPath} {
			logrus.Debugf("Watching file %s for changes", f)
			if err := watcher.Add(f); err != nil {
				logrus.Fatalf("Unable to watch %s: %v", f, err)
			}
		}
		<-done
	}()

	return reloader, nil
}

func (c *certReloader) reload() error {
	certificate, err := tls.LoadX509KeyPair(c.certPath, c.keyPath)
	if err != nil {
		return fmt.Errorf("load x509 key pair: %w", err)
	}
	if len(certificate.Certificate) == 0 {
		return errors.New("certificates chain is empty")
	}

	x509Cert, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		return fmt.Errorf("parse x509 certificate: %w", err)
	}
	logrus.Infof(
		"Metrics certificate is valid between %v and %v",
		x509Cert.NotBefore, x509Cert.NotAfter,
	)

	now := time.Now()
	if now.After(x509Cert.NotAfter) {
		return errors.New("certificate is not valid any more")
	}
	if now.Before(x509Cert.NotBefore) {
		return errors.New("certificate is not yet valid")
	}

	c.certLock.Lock()
	c.certificate = &certificate
	c.certLock.Unlock()

	return nil
}

func (c *certReloader) getCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	c.certLock.RLock()
	defer c.certLock.RUnlock()
	return c.certificate, nil
}
