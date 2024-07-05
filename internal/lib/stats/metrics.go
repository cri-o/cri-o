package statsserver

import (
	"time"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
)

var baseLabelKeys = []string{"id", "name", "image"}

const NetworkMetrics = "network"

type metricValue struct {
	value      uint64
	labels     []string
	metricType types.MetricType
}

type metricValues []metricValue

type containerMetric struct {
	desc      *types.MetricDescriptor
	valueFunc func() metricValues
}

type SandboxMetrics struct {
	metric *types.PodSandboxMetrics
}

func (s *SandboxMetrics) GetMetric() *types.PodSandboxMetrics {
	return s.metric
}

func NewSandboxMetrics(sb *sandbox.Sandbox) *SandboxMetrics {
	return &SandboxMetrics{
		metric: &types.PodSandboxMetrics{
			PodSandboxId:     sb.ID(),
			Metrics:          []*types.Metric{},
			ContainerMetrics: []*types.ContainerMetrics{},
		},
	}
}

// PopulateMetricDescriptors stores metricdescriptors statically at startup and populates the list.
func (ss *StatsServer) PopulateMetricDescriptors(includedKeys []string) map[string][]*types.MetricDescriptor {
	descriptorsMap := map[string][]*types.MetricDescriptor{
		"cpu": {
			{
				Name:      "container_cpu_user_seconds_total",
				Help:      "Cumulative user CPU time consumed in seconds.",
				LabelKeys: baseLabelKeys,
			}, {
				Name:      "container_cpu_system_seconds_total",
				Help:      "Cumulative system CPU time consumed in seconds.",
				LabelKeys: baseLabelKeys,
			}, {
				Name:      "container_cpu_usage_seconds_total",
				Help:      "Cumulative CPU time consumed in seconds.",
				LabelKeys: append(baseLabelKeys, "cpu"),
			}, {
				Name:      "container_cpu_cfs_periods_total",
				Help:      "Number of elapsed enforcement period intervals.",
				LabelKeys: baseLabelKeys,
			}, {
				Name:      "container_cpu_cfs_throttled_periods_total",
				Help:      "Number of throttled period intervals.",
				LabelKeys: baseLabelKeys,
			}, {
				Name:      "container_cpu_cfs_throttled_seconds_total",
				Help:      "Total time duration the container has been throttled.",
				LabelKeys: baseLabelKeys,
			},
		},
		"cpuLoad": {
			{
				Name:      "container_cpu_load_average_10s",
				Help:      "Value of container CPU load average over the last 10 seconds.",
				LabelKeys: baseLabelKeys,
			}, {
				Name:      "container_tasks_state",
				Help:      "Number of tasks in given state",
				LabelKeys: append(baseLabelKeys, "state"),
			},
		},
		"disk": {
			{
				Name:      "container_fs_inodes_free",
				Help:      "Number of available Inodes",
				LabelKeys: append(baseLabelKeys, "device"),
			}, {
				Name:      "container_fs_inodes_total",
				Help:      "Number of Inodes",
				LabelKeys: append(baseLabelKeys, "device"),
			}, {
				Name:      "container_fs_limit_bytes",
				Help:      "Number of bytes that can be consumed by the container on this filesystem.",
				LabelKeys: append(baseLabelKeys, "device"),
			}, {
				Name:      "container_fs_usage_bytes",
				Help:      "Number of bytes that are consumed by the container on this filesystem.",
				LabelKeys: append(baseLabelKeys, "device"),
			},
		},
		"diskio": {
			{
				Name:      "container_fs_reads_bytes_total",
				Help:      "Cumulative count of bytes read",
				LabelKeys: append(baseLabelKeys, "device"),
			},
			{
				Name:      "container_fs_reads_total",
				Help:      "Cumulative count of reads completed",
				LabelKeys: append(baseLabelKeys, "device"),
			},
			{
				Name:      "container_fs_sector_reads_total",
				Help:      "Cumulative count of sector reads completed",
				LabelKeys: append(baseLabelKeys, "device"),
			},
			{
				Name:      "container_fs_reads_merged_total",
				Help:      "Cumulative count of reads merged",
				LabelKeys: append(baseLabelKeys, "device"),
			},
			{
				Name:      "container_fs_read_seconds_total",
				Help:      "Cumulative count of seconds spent reading",
				LabelKeys: append(baseLabelKeys, "device"),
			},
			{
				Name:      "container_fs_writes_bytes_total",
				Help:      "Cumulative count of bytes written",
				LabelKeys: append(baseLabelKeys, "device"),
			},
			{
				Name:      "container_fs_writes_total",
				Help:      "Cumulative count of writes completed",
				LabelKeys: append(baseLabelKeys, "device"),
			},
			{
				Name:      "container_fs_sector_writes_total",
				Help:      "Cumulative count of sector writes completed",
				LabelKeys: append(baseLabelKeys, "device"),
			},
			{
				Name:      "container_fs_writes_merged_total",
				Help:      "Cumulative count of writes merged",
				LabelKeys: append(baseLabelKeys, "device"),
			},
			{
				Name:      "container_fs_write_seconds_total",
				Help:      "Cumulative count of seconds spent writing",
				LabelKeys: append(baseLabelKeys, "device"),
			},
			{
				Name:      "container_fs_io_current",
				Help:      "Number of I/Os currently in progress",
				LabelKeys: append(baseLabelKeys, "device"),
			},
			{
				Name:      "container_fs_io_time_seconds_total",
				Help:      "Cumulative count of seconds spent doing I/Os",
				LabelKeys: append(baseLabelKeys, "device"),
			},
			{
				Name:      "container_fs_io_time_weighted_seconds_total",
				Help:      "Cumulative weighted I/O time in seconds",
				LabelKeys: append(baseLabelKeys, "device"),
			},
			{
				Name:      "container_blkio_device_usage_total",
				Help:      "Blkio device bytes usage",
				LabelKeys: append(baseLabelKeys, "device", "major", "minor", "operation"),
			},
		},
		"memory": {
			{
				Name:      "container_memory_cache",
				Help:      "Number of bytes of page cache memory.",
				LabelKeys: baseLabelKeys,
			},
			{
				Name:      "container_memory_rss",
				Help:      "Size of RSS in bytes.",
				LabelKeys: baseLabelKeys,
			},
			{
				Name:      "container_memory_kernel_usage",
				Help:      "Size of kernel memory allocated in bytes.",
				LabelKeys: baseLabelKeys,
			},
			{
				Name:      "container_memory_mapped_file",
				Help:      "Size of memory mapped files in bytes.",
				LabelKeys: baseLabelKeys,
			},
			{
				Name:      "container_memory_swap",
				Help:      "Container swap usage in bytes.",
				LabelKeys: baseLabelKeys,
			},
			{
				Name:      "container_memory_failcnt",
				Help:      "Number of memory usage hits limits",
				LabelKeys: baseLabelKeys,
			},
			{
				Name:      "container_memory_usage_bytes",
				Help:      "Current memory usage in bytes, including all memory regardless of when it was accessed",
				LabelKeys: baseLabelKeys,
			},
			{
				Name:      "container_memory_max_usage_bytes",
				Help:      "Maximum memory usage recorded in bytes",
				LabelKeys: baseLabelKeys,
			},
			{
				Name:      "container_memory_working_set_bytes",
				Help:      "Current working set in bytes.",
				LabelKeys: baseLabelKeys,
			},
			{
				Name:      "container_memory_failures_total",
				Help:      "Cumulative count of memory allocation failures.",
				LabelKeys: append(baseLabelKeys, "failure_type", "scope"),
			},
		},
		"misc": {
			{
				Name:      "container_scrape_error",
				Help:      "1 if there was an error while getting container metrics, 0 otherwise",
				LabelKeys: baseLabelKeys,
			}, {
				Name:      "container_last_seen",
				Help:      "Last time a container was seen by the exporter",
				LabelKeys: baseLabelKeys,
			}, {
				Name:      "cadvisor_version_info",
				Help:      "A metric with a constant '1' value labeled by kernel version, OS version, docker version, cadvisor version & cadvisor revision.",
				LabelKeys: []string{"kernelVersion", "osVersion", "dockerVersion", "cadvisorVersion", "cadvisorRevision"},
			}, {
				Name:      "container_start_time_seconds",
				Help:      "Start time of the container since unix epoch in seconds.",
				LabelKeys: baseLabelKeys,
			}, {
				Name:      "container_spec_cpu_period",
				Help:      "CPU period of the container.",
				LabelKeys: baseLabelKeys,
			}, {
				Name:      "container_spec_cpu_quota",
				Help:      "CPU quota of the container.",
				LabelKeys: baseLabelKeys,
			}, {
				Name:      "container_spec_cpu_shares",
				Help:      "CPU share of the container.",
				LabelKeys: baseLabelKeys,
			},
		},
		NetworkMetrics: {
			{
				Name:      "container_network_receive_bytes_total",
				Help:      "Cumulative count of bytes received",
				LabelKeys: append(baseLabelKeys, "interface"),
			}, {
				Name:      "container_network_receive_packets_total",
				Help:      "Cumulative count of packets received",
				LabelKeys: append(baseLabelKeys, "interface"),
			}, {
				Name:      "container_network_receive_packets_dropped_total",
				Help:      "Cumulative count of packets dropped while receiving",
				LabelKeys: append(baseLabelKeys, "interface"),
			}, {
				Name:      "container_network_receive_errors_total",
				Help:      "Cumulative count of errors encountered while receiving",
				LabelKeys: append(baseLabelKeys, "interface"),
			}, {
				Name:      "container_network_transmit_bytes_total",
				Help:      "Cumulative count of bytes transmitted",
				LabelKeys: append(baseLabelKeys, "interface"),
			}, {
				Name:      "container_network_transmit_packets_total",
				Help:      "Cumulative count of packets transmitted",
				LabelKeys: append(baseLabelKeys, "interface"),
			}, {
				Name:      "container_network_transmit_packets_dropped_total",
				Help:      "Cumulative count of packets dropped while transmitting",
				LabelKeys: append(baseLabelKeys, "interface"),
			}, {
				Name:      "container_network_transmit_errors_total",
				Help:      "Cumulative count of errors encountered while transmitting",
				LabelKeys: append(baseLabelKeys, "interface"),
			},
		},
		"oom": {
			{
				Name:      "container_oom_events_total",
				Help:      "Count of out of memory events observed for the container",
				LabelKeys: baseLabelKeys,
			},
		},
		"processes": {
			{
				Name:      "container_processes",
				Help:      "Number of processes running inside the container.",
				LabelKeys: baseLabelKeys,
			},
			{
				Name:      "container_file_descriptors",
				Help:      "Number of open file descriptors for the container.",
				LabelKeys: baseLabelKeys,
			},
			{
				Name:      "container_sockets",
				Help:      "Number of open sockets for the container.",
				LabelKeys: baseLabelKeys,
			},
			{
				Name:      "container_threads_max",
				Help:      "Maximum number of threads allowed inside the container, infinity if value is zero",
				LabelKeys: baseLabelKeys,
			},
			{
				Name:      "container_threads",
				Help:      "Number of threads running inside the container",
				LabelKeys: baseLabelKeys,
			},
			{
				Name:      "container_ulimits_soft",
				Help:      "Soft ulimit values for the container root process. Unlimited if -1, except priority and nice",
				LabelKeys: append(baseLabelKeys, "ulimit"),
			},
		},
	}
	return descriptorsMap
}

func sandboxBaseLabelValues(sb *sandbox.Sandbox) []string {
	// TODO FIXME: image?
	return []string{sb.ID(), "POD", ""}
}

// ComputeSandboxMetrics computes the metrics for both pod and container sandbox.
func computeSandboxMetrics(sb *sandbox.Sandbox, metrics []*containerMetric, metricName string) []*types.Metric {
	values := append(sandboxBaseLabelValues(sb), metricName)
	calculatedMetrics := make([]*types.Metric, 0, len(metrics))

	for _, m := range metrics {
		for _, v := range m.valueFunc() {
			newMetric := &types.Metric{
				Name:        m.desc.Name,
				Timestamp:   time.Now().UnixNano(),
				MetricType:  v.metricType,
				Value:       &types.UInt64Value{Value: v.value},
				LabelValues: append(values, v.labels...),
			}
			calculatedMetrics = append(calculatedMetrics, newMetric)
		}
	}

	return calculatedMetrics
}
