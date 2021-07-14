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
	Operations Collector = crioPrefix + "operations"

	// OperationsLatencyTotal is the key for the operation latency metrics.
	OperationsLatencyTotal Collector = crioPrefix + "operations_latency_microseconds_total"

	// OperationsLatency is the key for the operation latency metrics for each CRI call.
	OperationsLatency Collector = crioPrefix + "operations_latency_microseconds"

	// OperationsErrors is the key for the operation error metrics.
	OperationsErrors Collector = crioPrefix + "operations_errors"

	// ImagePullsByDigest is the key for CRI-O image pull metrics by digest.
	ImagePullsByDigest Collector = crioPrefix + "image_pulls_by_digest"

	// ImagePullsByName is the key for CRI-O image pull metrics by name.
	ImagePullsByName Collector = crioPrefix + "image_pulls_by_name"

	// ImagePullsByNameSkipped is the key for CRI-O skipped image pull metrics by name (skipped).
	ImagePullsByNameSkipped Collector = crioPrefix + "image_pulls_by_name_skipped"

	// ImagePullsFailures is the key for failed image downloads in CRI-O.
	ImagePullsFailures Collector = crioPrefix + "image_pulls_failures"

	// ImagePullsSuccesses is the key for successful image downloads in CRI-O.
	ImagePullsSuccesses Collector = crioPrefix + "image_pulls_successes"

	// ImagePullsLayerSize is the key for CRI-O image pull metrics per layer.
	ImagePullsLayerSize Collector = crioPrefix + "image_pulls_layer_size"

	// ImageLayerReuse is the key for the CRI-O image layer reuse metrics.
	ImageLayerReuse Collector = crioPrefix + "image_layer_reuse"

	// ContainersOOMTotal is the key for the total CRI-O container out of memory metrics.
	ContainersOOMTotal Collector = crioPrefix + "containers_oom_total"

	// ContainersOOM is the key for the CRI-O container out of memory metrics per container name.
	ContainersOOM Collector = crioPrefix + "containers_oom"
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
		Operations.Stripped(),
		OperationsLatencyTotal.Stripped(),
		OperationsLatency.Stripped(),
		OperationsErrors.Stripped(),
		ImagePullsByDigest.Stripped(),
		ImagePullsByName.Stripped(),
		ImagePullsByNameSkipped.Stripped(),
		ImagePullsFailures.Stripped(),
		ImagePullsSuccesses.Stripped(),
		ImagePullsLayerSize.Stripped(),
		ImageLayerReuse.Stripped(),
		ContainersOOMTotal.Stripped(),
		ContainersOOM.Stripped(),
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
