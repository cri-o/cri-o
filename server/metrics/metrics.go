package metrics

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	libconfig "github.com/cri-o/cri-o/pkg/config"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

const (
	// CRIOOperationsKey is the key for CRI-O operation metrics.
	CRIOOperationsKey = "crio_operations"

	// CRIOOperationsLatencyTotalKey is the key for the operation latency metrics.
	CRIOOperationsLatencyTotalKey = "crio_operations_latency_microseconds_total"

	// CRIOOperationsLatencyKey is the key for the operation latency metrics for each CRI call.
	CRIOOperationsLatencyKey = "crio_operations_latency_microseconds"

	// CRIOOperationsErrorsKey is the key for the operation error metrics.
	CRIOOperationsErrorsKey = "crio_operations_errors"

	// CRIOImagePullsByDigestKey is the key for CRI-O image pull metrics by digest.
	CRIOImagePullsByDigestKey = "crio_image_pulls_by_digest"

	// CRIOImagePullsByNameKey is the key for CRI-O image pull metrics by name.
	CRIOImagePullsByNameKey = "crio_image_pulls_by_name"

	// CRIOImagePullsByNameSkippedKey is the key for CRI-O skipped image pull metrics by name (skipped).
	CRIOImagePullsByNameSkippedKey = "crio_image_pulls_by_name_skipped"

	// CRIOImagePullsFailuresKey is the key for failed image downloads in CRI-O.
	CRIOImagePullsFailuresKey = "crio_image_pulls_failures"

	// CRIOImagePullsSuccessesKey is the key for successful image downloads in CRI-O.
	CRIOImagePullsSuccessesKey = "crio_image_pulls_successes"

	// CRIOImageLayerReuseKey is the key for the CRI-O image layer reuse metrics.
	CRIOImageLayerReuseKey = "crio_image_layer_reuse"

	// CRIOContainersOOMTotalKey is the key for the total CRI-O container out of memory metrics.
	CRIOContainersOOMTotalKey = "crio_containers_oom_total"

	// CRIOContainersOOMKey is the key for the CRI-O container out of memory metrics per container name.
	CRIOContainersOOMKey = "crio_containers_oom"

	subsystem = "container_runtime"
)

var (
	// CRIOOperations collects operation counts by operation type.
	CRIOOperations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: subsystem,
			Name:      CRIOOperationsKey,
			Help:      "Cumulative number of CRI-O operations by operation type.",
		},
		[]string{"operation_type"},
	)

	// CRIOOperationsLatencyTotal collects operation latency numbers by operation
	// type.
	CRIOOperationsLatencyTotal = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Subsystem: subsystem,
			Name:      CRIOOperationsLatencyTotalKey,
			Help:      "Latency in microseconds of CRI-O operations. Broken down by operation type.",
		},
		[]string{"operation_type"},
	)

	// CRIOOperationsLatency collects operation latency numbers for each CRI call by operation
	// type.
	CRIOOperationsLatency = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: subsystem,
			Name:      CRIOOperationsLatencyKey,
			Help:      "Latency in microseconds of individual CRI calls for CRI-O operations. Broken down by operation type.",
		},
		[]string{"operation_type"},
	)

	// CRIOOperationsErrors collects operation errors by operation
	// type.
	CRIOOperationsErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: subsystem,
			Name:      CRIOOperationsErrorsKey,
			Help:      "Cumulative number of CRI-O operation errors by operation type.",
		},
		[]string{"operation_type"},
	)

	// CRIOImagePullsByDigest collects image pull metrics for every image digest
	CRIOImagePullsByDigest = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: subsystem,
			Name:      CRIOImagePullsByDigestKey,
			Help:      "Bytes transferred by CRI-O image pulls by digest",
		},
		[]string{"name", "digest", "mediatype", "size"},
	)

	// CRIOImagePullsByName collects image pull metrics for every image name
	CRIOImagePullsByName = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: subsystem,
			Name:      CRIOImagePullsByNameKey,
			Help:      "Bytes transferred by CRI-O image pulls by name",
		},
		[]string{"name", "size"},
	)

	// CRIOImagePullsByNameSkipped collects image pull metrics for every image name (skipped)
	CRIOImagePullsByNameSkipped = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: subsystem,
			Name:      CRIOImagePullsByNameSkippedKey,
			Help:      "Bytes skipped by CRI-O image pulls by name",
		},
		[]string{"name"},
	)

	// CRIOImagePullsFailures collects image pull failures
	CRIOImagePullsFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: subsystem,
			Name:      CRIOImagePullsFailuresKey,
			Help:      "Cumulative number of CRI-O image pull failures by error.",
		},
		[]string{"name", "error"},
	)

	// CRIOImagePullsSuccesses collects image pull successes
	CRIOImagePullsSuccesses = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: subsystem,
			Name:      CRIOImagePullsSuccessesKey,
			Help:      "Cumulative number of CRI-O image pull successes.",
		},
		[]string{"name"},
	)

	// CRIOImageLayerReuse collects image pull metrics for every resused image layer
	CRIOImageLayerReuse = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: subsystem,
			Name:      CRIOImageLayerReuseKey,
			Help:      "Reused (not pulled) local image layer count by name",
		},
		[]string{"name"},
	)

	// CRIOContainersOOMTotal collects container out of memory (oom) metrics for every container and sandboxes.
	CRIOContainersOOMTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Subsystem: subsystem,
			Name:      CRIOContainersOOMTotalKey,
			Help:      "Amount of containers killed because they ran out of memory (OOM)",
		},
	)

	// CRIOContainersOOM collects container out of memory (oom) metrics per container and sandbox name.
	CRIOContainersOOM = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: subsystem,
			Name:      CRIOContainersOOMKey,
			Help:      "Amount of containers killed because they ran out of memory (OOM) by their name",
		},
		[]string{"name"},
	)
)

// SinceInMicroseconds gets the time since the specified start in microseconds.
func SinceInMicroseconds(start time.Time) float64 {
	return float64(time.Since(start).Microseconds())
}

// Metrics is the main structure for starting the metrics endpoints.
type Metrics struct {
	config *libconfig.MetricsConfig
}

// New creates a new metrics instance.
func New(config *libconfig.MetricsConfig) *Metrics {
	return &Metrics{config}
}

// Start starts serving the metrics in the background.
func (m *Metrics) Start(stop chan struct{}) error {
	if m.config == nil {
		return errors.New("provided config is nil")
	}

	me, err := m.createEndpoint()
	if err != nil {
		return errors.Wrap(err, "create endpoint")
	}

	if err := m.startEndpoint(
		stop, "tcp", fmt.Sprintf(":%v", m.config.MetricsPort), me,
	); err != nil {
		return errors.Wrapf(
			err, "create metrics endpoint on port %d", m.config.MetricsPort,
		)
	}

	metricsSocket := m.config.MetricsSocket
	if metricsSocket != "" {
		if err := libconfig.RemoveUnusedSocket(metricsSocket); err != nil {
			return errors.Wrapf(err, "removing unused socket %s", metricsSocket)
		}

		return errors.Wrap(
			m.startEndpoint(stop, "unix", m.config.MetricsSocket, me),
			"creating metrics endpoint socket",
		)
	}

	return nil
}

// createEndpoint creates a /metrics endpoint for prometheus monitoring.
func (m *Metrics) createEndpoint() (*http.ServeMux, error) {
	for _, collector := range []prometheus.Collector{
		CRIOOperations,
		CRIOOperationsLatency,
		CRIOOperationsLatencyTotal,
		CRIOOperationsErrors,
		CRIOImagePullsByDigest,
		CRIOImagePullsByName,
		CRIOImagePullsByNameSkipped,
		CRIOImagePullsFailures,
		CRIOImagePullsSuccesses,
		CRIOImageLayerReuse,
		CRIOContainersOOMTotal,
		CRIOContainersOOM,
	} {
		if err := prometheus.Register(collector); err != nil {
			return nil, errors.Wrap(err, "register metric")
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
		return errors.Wrap(err, "creating listener")
	}

	go func() {
		var err error
		if m.config.MetricsCert != "" && m.config.MetricsKey != "" {
			logrus.Infof("Serving metrics on %s via HTTPs", address)

			kpr, reloadErr := newCertReloader(
				stop, m.config.MetricsCert, m.config.MetricsKey,
			)
			if reloadErr != nil {
				logrus.Fatalf("Creating key pair reloader: %v", reloadErr)
			}

			srv := http.Server{
				Handler: me,
				TLSConfig: &tls.Config{
					GetCertificate: kpr.getCertificate,
					MinVersion:     tls.VersionTLS12,
				},
			}
			err = srv.ServeTLS(l, m.config.MetricsCert, m.config.MetricsKey)
		} else {
			logrus.Infof("Serving metrics on %s via HTTP", address)
			err = http.Serve(l, me)
		}

		if err != nil {
			logrus.Fatalf("Failed to serve metrics endpoint %v: %v", l, err)
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

	if err := reloader.reload(); err != nil {
		return nil, errors.Wrap(err, "load certificate")
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, errors.Wrap(err, "create new watcher")
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
		return errors.Wrap(err, "load x509 key pair")
	}
	if len(certificate.Certificate) == 0 {
		return errors.New("certificates chain is empty")
	}

	x509Cert, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		return errors.Wrap(err, "parse x509 certificate")
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
