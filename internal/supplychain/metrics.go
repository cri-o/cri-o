package supplychain

import "github.com/prometheus/client_golang/prometheus"

var (
	// VerificationTotal counts supply chain verification attempts by type and result.
	VerificationTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "crio",
			Name:      "supply_chain_verification_total",
			Help:      "Total number of supply chain verification attempts.",
		},
		[]string{"type", "result"},
	)

	// VerificationDuration measures supply chain verification latency by type.
	VerificationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "crio",
			Name:      "supply_chain_verification_duration_seconds",
			Help:      "Duration of supply chain verification in seconds.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"type"},
	)

	// CacheHitsTotal counts cache hits for supply chain verification results.
	CacheHitsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "crio",
			Name:      "supply_chain_cache_hits_total",
			Help:      "Total number of supply chain verification cache hits.",
		},
	)

	// FetchErrorsTotal counts attestation fetch errors by type.
	// Currently unused; will be incremented once cosign attestation fetching is implemented.
	FetchErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "crio",
			Name:      "supply_chain_fetch_errors_total",
			Help:      "Total number of supply chain attestation fetch errors.",
		},
		[]string{"type"},
	)
)
