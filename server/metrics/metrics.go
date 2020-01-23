package metrics

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	// CRIOOperationsKey is the key for CRI-O operation metrics.
	CRIOOperationsKey = "crio_operations"

	// CRIOOperationsLatencyKey is the key for the operation latency metrics.
	CRIOOperationsLatencyKey = "crio_operations_latency_microseconds"

	// CRIOOperationsErrorsKey is the key for the operation error metrics.
	CRIOOperationsErrorsKey = "crio_operations_errors"

	// CRIOImagePullsByDigestKey is the key for CRI-O image pull metrics by digest.
	CRIOImagePullsByDigestKey = "crio_image_pulls_by_digest"

	// CRIOImagePullsByNameKey is the key for CRI-O image pull metrics by name.
	CRIOImagePullsByNameKey = "crio_image_pulls_by_name"

	// CRIOImagePullsByNameSkippedKey is the key for CRI-O skipped image pull metrics by name (skipped).
	CRIOImagePullsByNameSkippedKey = "crio_image_pulls_by_name_skipped"

	// TODO(runcom):
	// timeouts

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

	// CRIOOperationsLatency collects operation latency numbers by operation
	// type.
	CRIOOperationsLatency = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Subsystem: subsystem,
			Name:      CRIOOperationsLatencyKey,
			Help:      "Latency in microseconds of CRI-O operations. Broken down by operation type.",
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
)

var registerMetrics sync.Once

// Register all metrics
func Register() {
	registerMetrics.Do(func() {
		prometheus.MustRegister(CRIOOperations)
		prometheus.MustRegister(CRIOOperationsLatency)
		prometheus.MustRegister(CRIOOperationsErrors)
		prometheus.MustRegister(CRIOImagePullsByDigest)
		prometheus.MustRegister(CRIOImagePullsByName)
		prometheus.MustRegister(CRIOImagePullsByNameSkipped)
	})
}

// SinceInMicroseconds gets the time since the specified start in microseconds.
func SinceInMicroseconds(start time.Time) float64 {
	return float64(time.Since(start).Nanoseconds() / time.Microsecond.Nanoseconds())
}
