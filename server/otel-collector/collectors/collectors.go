package collectors

import "strings"

// Collector specifies a single metrics collector identifier.
type Collector string

// Collectors specifies a list of metrics collectors.
type Collectors []Collector

const (
	crioPrefix = "crio_"

	// Subsystem is the namespace where the metrics are being registered.
	Subsystem = "container_runtime"

	subsystemPrefix = Subsystem + "_"

	// Operations is the key for CRI-O operation metrics.
	// Deprecated: in favour of OperationsTotal
	Operations Collector = crioPrefix + "operations"

	// OperationsLatencyTotal is the key for the operation latency metrics.
	// Deprecated: in favour of OperationsLatencySecondsTotal
	OperationsLatencyTotal Collector = crioPrefix + "operations_latency_microseconds_total"

	// OperationsLatency is the key for the operation latency metrics for each CRI call.
	// Deprecated: in favour of OperationsLatencySeconds
	OperationsLatency Collector = crioPrefix + "operations_latency_microseconds"

	// OperationsErrors is the key for the operation error metrics.
	// Deprecated: in favour of OperationsErrorsTotal
	OperationsErrors Collector = crioPrefix + "operations_errors"

	// ImagePullsByDigest is the key for CRI-O image pull metrics by digest.
	// Deprecated: in favour of ImagePullsBytesTotal
	ImagePullsByDigest Collector = crioPrefix + "image_pulls_by_digest"

	// ImagePullsByName is the key for CRI-O image pull metrics by name.
	// Deprecated: in favour of ImagePullsBytesTotal
	ImagePullsByName Collector = crioPrefix + "image_pulls_by_name"

	// ImagePullsByNameSkipped is the key for CRI-O skipped image pull metrics by name (skipped).
	// Deprecated: in favour of ImagePullsSkippedBytesTotal
	ImagePullsByNameSkipped Collector = crioPrefix + "image_pulls_by_name_skipped"

	// ImagePullsFailures is the key for failed image downloads in CRI-O.
	// Deprecated: in favour of ImagePullsFailureTotal
	ImagePullsFailures Collector = crioPrefix + "image_pulls_failures"

	// ImagePullsSuccesses is the key for successful image downloads in CRI-O.
	// Deprecated: in favour of ImagePullsSuccessTotal
	ImagePullsSuccesses Collector = crioPrefix + "image_pulls_successes"

	// ImagePullsLayerSize is the key for CRI-O image pull metrics per layer.
	ImagePullsLayerSize Collector = crioPrefix + "image_pulls_layer_size"

	// ImageLayerReuse is the key for the CRI-O image layer reuse metrics.
	// Deprecated: in favour of ImageLayerReuseTotal
	ImageLayerReuse Collector = crioPrefix + "image_layer_reuse"

	// ContainersEventsDropped is the key for the total number of container events dropped counter.
	ContainersEventsDropped Collector = crioPrefix + "containers_events_dropped_total"

	// ContainersOOMTotal is the key for the total CRI-O container out of memory metrics.
	ContainersOOMTotal Collector = crioPrefix + "containers_oom_total"

	// ContainersOOM is the key for the CRI-O container out of memory metrics per container name.
	// Deprecated: in favour of ContainersOOMCountTotal
	ContainersOOM Collector = crioPrefix + "containers_oom"

	// ProcessesDefunct is the key for the total number of defunct processes in a node.
	ProcessesDefunct Collector = crioPrefix + "processes_defunct"

	// OperationsTotal is the key for CRI-O operation metrics.
	OperationsTotal Collector = crioPrefix + "operations_total"

	// OperationsLatencySeconds is the key for the operation latency metrics for each CRI call.
	OperationsLatencySeconds Collector = crioPrefix + "operations_latency_seconds"

	// OperationsLatencySecondsTotal is the key for the operation latency metrics.
	OperationsLatencySecondsTotal Collector = crioPrefix + "operations_latency_seconds_total"

	// OperationsErrorsTotal is the key for the operation error metrics.
	OperationsErrorsTotal Collector = crioPrefix + "operations_errors_total"

	// ImagePullsBytesTotal is the key for CRI-O image pull metrics.
	ImagePullsBytesTotal Collector = crioPrefix + "image_pulls_bytes_total"

	// ImagePullsSkippedBytesTotal is the key for CRI-O skipped image pull metrics.
	ImagePullsSkippedBytesTotal Collector = crioPrefix + "image_pulls_skipped_bytes_total"

	// ImagePullsFailureTotal is the key for failed image downloads in CRI-O.
	ImagePullsFailureTotal Collector = crioPrefix + "image_pulls_failure_total"

	// ImagePullsSuccessTotal is the key for successful image downloads in CRI-O.
	ImagePullsSuccessTotal Collector = crioPrefix + "image_pulls_success_total"

	// ImageLayerReuseTotal is the key for the CRI-O image layer reuse metrics.
	ImageLayerReuseTotal Collector = crioPrefix + "image_layer_reuse_total"

	// ContainersOOMCountTotal is the key for the CRI-O container out of memory metrics per container name.
	ContainersOOMCountTotal Collector = crioPrefix + "containers_oom_count_total"

	// ContainersSeccompNotifierCountTotal is the key for the CRI-O container seccomp notifier metrics per container name and syscalls.
	ContainersSeccompNotifierCountTotal Collector = crioPrefix + "containers_seccomp_notifier_count_total"

	// ResourcesStalledAtStage is the key for the resources stalled at different stages in container and pod creation.
	ResourcesStalledAtStage Collector = crioPrefix + "resources_stalled_at_stage"
)

// FromSlice converts a string slice to a Collectors type.
func FromSlice(in []string) (c Collectors) {
	for _, i := range in {
		c = append(c, Collector(i).Stripped())
	}
	return c
}

// ToSlice converts a Collectors type to a string slice.
func (c Collectors) ToSlice() (r []string) {
	for _, i := range c {
		r = append(r, i.Stripped().String())
	}
	return r
}

// All returns all available metrics collectors referenced by their
// name key.
func All() Collectors {
	return Collectors{
		Operations.Stripped(),              // Deprecated: in favour of OperationsTotal
		OperationsLatencyTotal.Stripped(),  // Deprecated: in favour of OperationsLatencySecondsTotal
		OperationsLatency.Stripped(),       // Deprecated: in favour of OperationsLatencySeconds
		OperationsErrors.Stripped(),        // Deprecated: in favour of OperationsErrorsTotal
		ImagePullsByDigest.Stripped(),      // Deprecated: in favour of ImagePullsBytesTotal
		ImagePullsByName.Stripped(),        // Deprecated: in favour of ImagePullsBytesTotal
		ImagePullsByNameSkipped.Stripped(), // Deprecated: in favour of ImagePullsSkippedBytesTotal
		ImagePullsFailures.Stripped(),      // Deprecated: in favour of ImagePullsFailureTotal
		ImagePullsSuccesses.Stripped(),     // Deprecated: in favour of ImagePullsSuccessTotal
		ImagePullsLayerSize.Stripped(),
		ImageLayerReuse.Stripped(), // Deprecated: in favour of ImageLayerReuseTotal
		ContainersEventsDropped.Stripped(),
		ContainersOOMTotal.Stripped(),
		ContainersOOM.Stripped(), // Deprecated: in favour of ContainersOOMCountTotal
		ProcessesDefunct.Stripped(),
		OperationsTotal.Stripped(),
		OperationsLatencySeconds.Stripped(),
		OperationsLatencySecondsTotal.Stripped(),
		OperationsErrorsTotal.Stripped(),
		ImagePullsBytesTotal.Stripped(),
		ImagePullsSkippedBytesTotal.Stripped(),
		ImagePullsFailureTotal.Stripped(),
		ImagePullsSuccessTotal.Stripped(),
		ImageLayerReuseTotal.Stripped(),
		ContainersOOMCountTotal.Stripped(),
		ContainersSeccompNotifierCountTotal.Stripped(),
		ResourcesStalledAtStage.Stripped(),
	}
}

// Contains returns true if the provided Collector `in` is part of the
// collectors instance.
func (c Collectors) Contains(in Collector) bool {
	stripped := in.Stripped()
	for _, collector := range c {
		if stripped == collector.Stripped() {
			return true
		}
	}

	return false
}

// stripPrefix strips the metrics prefixes from the provided string.
func stripPrefix(s string) string {
	s = strings.TrimPrefix(s, subsystemPrefix)
	return strings.TrimPrefix(s, crioPrefix)
}

// Stripped returns a prefix stripped name for the collector.
func (c Collector) Stripped() Collector {
	return Collector(stripPrefix(c.String()))
}

// String returns a string for the collector.
func (c Collector) String() string {
	return string(c)
}
