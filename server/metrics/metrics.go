package metrics

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"

	"github.com/cri-o/cri-o/internal/cert"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/process"
	"github.com/cri-o/cri-o/internal/storage/references"
	libconfig "github.com/cri-o/cri-o/pkg/config"
	"github.com/cri-o/cri-o/server/metrics/collectors"
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
	metricImagePullsLayerSize                 prometheus.Histogram
	metricContainersEventsDropped             prometheus.Counter
	metricContainersOOMTotal                  prometheus.Counter
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
	metricContainersStoppedMonitorCount       *prometheus.CounterVec
}

var instance *Metrics

// New creates a new metrics instance.
func New(config *libconfig.MetricsConfig) *Metrics {
	instance = &Metrics{
		config: config,
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
		metricContainersStoppedMonitorCount: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: collectors.Subsystem,
				Name:      collectors.ContainersStoppedMonitorCount.String(),
				Help:      "Amount of containers whose monitor process has exited by their name",
			},
			[]string{"name"},
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
func (m *Metrics) Start(ctx context.Context, stop chan struct{}) error {
	if m.config == nil {
		return errors.New("provided config is nil")
	}

	me, err := m.createEndpoint()
	if err != nil {
		return fmt.Errorf("create endpoint: %w", err)
	}

	metricsAddress := net.JoinHostPort(m.config.MetricsHost, strconv.Itoa(m.config.MetricsPort))
	if err := m.startEndpoint(ctx, stop, "tcp", metricsAddress, me); err != nil {
		return fmt.Errorf("create metrics endpoint on %s: %w", metricsAddress, err)
	}

	metricsSocket := m.config.MetricsSocket
	if metricsSocket != "" {
		if err := libconfig.RemoveUnusedSocket(metricsSocket); err != nil {
			return fmt.Errorf("removing unused socket %s: %w", metricsSocket, err)
		}

		if err := m.startEndpoint(ctx, stop, "unix", m.config.MetricsSocket, me); err != nil {
			return fmt.Errorf("creating metrics endpoint socket: %w", err)
		}

		return nil
	}

	return nil
}

func (m *Metrics) MetricOperationsInc(operation string) {
	c, err := m.metricOperationsTotal.GetMetricWithLabelValues(operation)
	if err != nil {
		logrus.Warnf("Unable to write operations metric: %v", err)

		return
	}

	c.Inc()
}

func (m *Metrics) MetricOperationsLatencySet(operation string, start time.Time) {
	g, err := m.metricOperationsLatencySeconds.GetMetricWithLabelValues(operation)
	if err != nil {
		logrus.Warnf("Unable to write operation latency metric: %v", err)

		return
	}

	g.Set(SinceInSeconds(start))
}

func (m *Metrics) MetricOperationsLatencyTotalObserve(operation string, start time.Time) {
	o, err := m.metricOperationsLatencySecondsTotal.GetMetricWithLabelValues(operation)
	if err != nil {
		logrus.Warnf("Unable to write operation latency (total) metric: %v", err)

		return
	}

	o.Observe(SinceInSeconds(start))
}

func (m *Metrics) MetricOperationsErrorsInc(operation string) {
	c, err := m.metricOperationsErrorsTotal.GetMetricWithLabelValues(operation)
	if err != nil {
		logrus.Warnf("Unable to write operation errors metric: %v", err)

		return
	}

	c.Inc()
}

func (m *Metrics) MetricContainersOOMCountTotalInc(name string) {
	c, err := m.metricContainersOOMCountTotal.GetMetricWithLabelValues(name)
	if err != nil {
		logrus.Warnf("Unable to write container OOM metric: %v", err)

		return
	}

	c.Inc()
}

func (m *Metrics) MetricContainersOOMCountTotalDelete(name string) {
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

func (m *Metrics) MetricImagePullsSkippedBytesAdd(add float64) {
	c, err := m.metricImagePullsSkippedBytesTotal.GetMetricWithLabelValues(GetSizeBucket(add))
	if err != nil {
		logrus.Warnf("Unable to write image pulls skipped bytes metric: %v", err)

		return
	}

	c.Add(add)
}

func (m *Metrics) MetricImagePullsFailuresInc(image references.RegistryImageReference, label string) {
	c, err := m.metricImagePullsFailureTotal.GetMetricWithLabelValues(label)
	if err != nil {
		logrus.Warnf("Unable to write image pull failures total metric: %v", err)

		return
	}

	c.Inc()
}

func (m *Metrics) MetricImageLayerReuseInc(layer string) {
	c, err := m.metricImageLayerReuseTotal.GetMetricWithLabelValues(layer)
	if err != nil {
		logrus.Warnf("Unable to write image layer reuse total metric: %v", err)

		return
	}

	c.Inc()
}

func (m *Metrics) MetricImagePullsSuccessesInc(name references.RegistryImageReference) {
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

func (m *Metrics) MetricResourcesStalledAtStage(stage string) {
	c, err := m.metricResourcesStalledAtStage.GetMetricWithLabelValues(stage)
	if err != nil {
		logrus.Warnf("Unable to write resource stalled at stage metric: %v", err)

		return
	}

	c.Inc()
}

func (m *Metrics) MetricContainersStoppedMonitorCountInc(name string) {
	c, err := m.metricContainersStoppedMonitorCount.GetMetricWithLabelValues(name)
	if err != nil {
		logrus.Warnf("Unable to write container stopped monitor count metric: %v", err)

		return
	}

	c.Inc()
}

// createEndpoint creates a /metrics endpoint for prometheus monitoring.
func (m *Metrics) createEndpoint() (*http.ServeMux, error) {
	for collector, metric := range map[collectors.Collector]prometheus.Collector{
		collectors.ContainersEventsDropped:             m.metricContainersEventsDropped,
		collectors.ContainersOOMCountTotal:             m.metricContainersOOMCountTotal,
		collectors.ContainersOOMTotal:                  m.metricContainersOOMTotal,
		collectors.ContainersSeccompNotifierCountTotal: m.metricContainersSeccompNotifierCountTotal,
		collectors.ImageLayerReuseTotal:                m.metricImageLayerReuseTotal,
		collectors.ImagePullsBytesTotal:                m.metricImagePullsBytesTotal,
		collectors.ImagePullsFailureTotal:              m.metricImagePullsFailureTotal,
		collectors.ImagePullsLayerSize:                 m.metricImagePullsLayerSize,
		collectors.ImagePullsSkippedBytesTotal:         m.metricImagePullsSkippedBytesTotal,
		collectors.ImagePullsSuccessTotal:              m.metricImagePullsSuccessTotal,
		collectors.OperationsErrorsTotal:               m.metricOperationsErrorsTotal,
		collectors.OperationsLatencySeconds:            m.metricOperationsLatencySeconds,
		collectors.OperationsLatencySecondsTotal:       m.metricOperationsLatencySecondsTotal,
		collectors.OperationsTotal:                     m.metricOperationsTotal,
		collectors.ProcessesDefunct:                    m.metricProcessesDefunct,
		collectors.ResourcesStalledAtStage:             m.metricResourcesStalledAtStage,
		collectors.ContainersStoppedMonitorCount:       m.metricContainersStoppedMonitorCount,
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
	ctx context.Context, stop chan struct{}, network, address string, me http.Handler,
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
			log.Infof(ctx, "Serving metrics on %s using HTTPS", address)

			if err = cert.GenerateSelfSignedCertKey(ctx, m.config.MetricsCert, m.config.MetricsKey); err != nil {
				log.Fatalf(ctx, "Generating self-signed cert/key: %v", err)
			}

			var cc *cert.Config

			cc, err = cert.NewCertConfig(ctx, stop, m.config.MetricsCert, m.config.MetricsKey, "")
			if err != nil {
				log.Fatalf(ctx, "Creating key pair reloader: %v", err)
			}

			srv.TLSConfig = &tls.Config{
				GetConfigForClient: cc.GetConfigForClient,
				MinVersion:         tls.VersionTLS12,
			}

			go func() {
				<-stop

				if err := srv.Shutdown(ctx); err != nil {
					log.Errorf(ctx, "Error on metrics server shutdown: %v", err)
				}
			}()

			err = srv.ServeTLS(l, m.config.MetricsCert, m.config.MetricsKey)
		} else {
			log.Infof(ctx, "Serving metrics on %s using HTTP", address)

			go func() {
				<-stop

				if err := srv.Shutdown(ctx); err != nil {
					log.Errorf(ctx, "Error on metrics server shutdown: %v", err)
				}
			}()

			err = srv.Serve(l)
		}

		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf(ctx, "Failed to serve metrics endpoint %v: %v", l, err)
		}
	}()

	return nil
}
