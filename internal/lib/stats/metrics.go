package statsserver

import (
	"reflect"
	"time"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

var baseLabelKeys = []string{"id", "name", "image"}

type metricValue struct {
	value      uint64
	labels     []string
	metricType types.MetricType
}

type metricValues []metricValue

type ContainerStats struct {
	desc      *types.MetricDescriptor
	valueFunc func(stats interface{}) metricValues
}

type SandboxMetrics struct {
	current *types.PodSandboxMetrics
	next    *types.PodSandboxMetrics
}

func (s *SandboxMetrics) GetCurrent() *types.PodSandboxMetrics {
	return s.current
}

func (s *SandboxMetrics) GetNext() *types.PodSandboxMetrics {
	return s.next
}

func NewSandboxMetrics(sb *sandbox.Sandbox) *SandboxMetrics {
	return &SandboxMetrics{
		current: &types.PodSandboxMetrics{
			PodSandboxId:     sb.ID(),
			Metrics:          []*types.Metric{}, // TODO population function
			ContainerMetrics: []*types.ContainerMetrics{},
		},
		next: &types.PodSandboxMetrics{
			PodSandboxId:     sb.ID(),
			Metrics:          []*types.Metric{}, // Initialize as needed
			ContainerMetrics: []*types.ContainerMetrics{},
		},
	}
}

func (sm *SandboxMetrics) ResetMetricsForSandbox() {
	sm.next.Metrics = []*types.Metric{} // Reset metrics for the next iteration
}

func (sm *SandboxMetrics) AddMetricToSandbox(m *types.Metric) {
	sm.next.Metrics = append(sm.next.Metrics, m)
}

// store metricdescriptors statically at startup, populate the list
func (ss *StatsServer) PopulateMetricDescriptors(includedKeys []string) map[string][]*types.MetricDescriptor {
	// TODO: add default container labels
	descriptorsMap := map[string][]*types.MetricDescriptor{
		"cpu": {
			{
				Name:      "container_cpu_user_seconds_total", // stats.CpuStats.CpuUsage.UsageInUsermode (converted from nano)
				Help:      "Cumulative user cpu time consumed in seconds.",
				LabelKeys: baseLabelKeys,
			}, {
				Name:      "container_cpu_system_seconds_total", // stats.CpuStats.CpuUsage.UsageInKernelmode (converted from nano)
				Help:      "Cumulative system cpu time consumed in seconds.",
				LabelKeys: baseLabelKeys,
			}, {
				Name:      "container_cpu_usage_seconds_total", // stats.CpuStats.CpuUsage.TotalUsage (converted from nano)
				Help:      "Cumulative cpu time consumed in seconds.",
				LabelKeys: append(baseLabelKeys, "cpu"),
			}, {
				Name:      "container_cpu_cfs_periods_total", // stats.CpuStats.ThrottlingData.Periods
				Help:      "Number of elapsed enforcement period intervals.",
				LabelKeys: baseLabelKeys,
			}, {
				Name:      "container_cpu_cfs_throttled_periods_total", // stats.CpuStats.ThrottlingData.ThrottledPeriods
				Help:      "Number of throttled period intervals.",
				LabelKeys: baseLabelKeys,
			}, {
				Name:      "container_cpu_cfs_throttled_seconds_total", // stats.CpuStats.ThrottlingData.ThrottledTime (converted from nano)
				Help:      "Total time duration the container has been throttled.",
				LabelKeys: baseLabelKeys,
			},
		},
		"cpuLoad": {
			{
				Name:      "container_cpu_load_average_10s",
				Help:      "Value of container cpu load average over the last 10 seconds.",
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
				Help:      "Blkio Device bytes usage",
				LabelKeys: append(baseLabelKeys, "device", "major", "minor", "operation"),
			},
		},
		"memory": {
			{
				Name:      "container_memory_cache", // stats.MemoryStats.Cache
				Help:      "Number of bytes of page cache memory.",
				LabelKeys: baseLabelKeys,
			}, {
				Name:      "container_memory_rss",
				Help:      "Size of RSS in bytes.",
				LabelKeys: baseLabelKeys,
			},
			{
				Name:      "container_memory_kernel_usage",
				Help:      "Size of kernel memory allocated in bytes.",
				LabelKeys: baseLabelKeys,
			}, {
				Name:      "container_memory_mapped_file", // ??? TODO FIXME
				Help:      "Size of memory mapped files in bytes.",
				LabelKeys: baseLabelKeys,
			}, {
				Name:      "container_memory_swap", // stats.MemoryStats.SwapUsage.Usage
				Help:      "Container swap usage in bytes.",
				LabelKeys: baseLabelKeys,
			}, {
				Name:      "container_memory_failcnt", // stats.MemoryStats.Usage.Failcnt
				Help:      "Number of memory usage hits limits",
				LabelKeys: baseLabelKeys,
			}, {
				Name:      "container_memory_usage_bytes", // TODO FIXME
				Help:      "Current memory usage in bytes, including all memory regardless of when it was accessed",
				LabelKeys: baseLabelKeys,
			}, {
				Name:      "container_memory_max_usage_bytes", // stats.MemoryStats.Usage.MaxUsage
				Help:      "Maximum memory usage recorded in bytes",
				LabelKeys: baseLabelKeys,
			}, {
				Name:      "container_memory_working_set_bytes", // TODO FIXME
				Help:      "Current working set in bytes.",
				LabelKeys: baseLabelKeys,
			}, {
				Name:      "container_memory_failures_total", // TODO FIXME
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
		"network": {
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

func ComputeSandboxMetrics(sb *sandbox.Sandbox, stats interface{}, container []ContainerStats, metric string, sm *SandboxMetrics) []*types.Metric {
	values := append(sandboxBaseLabelValues(sb), metric)
	metrics := make([]*types.Metric, 0, len(container))
	for _, m := range container {
		metricValues := m.valueFunc(stats)
		if len(metricValues) == 0 {
			// No metrics to process for this ContainerStats, move to the next one
			continue
		}

		for _, v := range metricValues {
			// Check if a metric with the same label values already exists in SandboxMetrics
			existingMetric := findExistingMetric(sm.GetCurrent().Metrics, m.desc.Name, values, v.labels)
			if existingMetric != nil {
				// Reuse the existing metric by updating its value
				existingMetric.Value = &types.UInt64Value{Value: v.value}
			} else {
				metric := &types.Metric{
					Name:        m.desc.Name,
					Timestamp:   time.Now().UnixNano(),
					MetricType:  v.metricType,
					Value:       &types.UInt64Value{Value: v.value},
					LabelValues: append(values, v.labels...),
				}
				metrics = append(metrics, metric)
			}
		}
	}
	return metrics
}

// findExistingMetric finds an existing metric with the same label values.
func findExistingMetric(metrics []*types.Metric, name string, values, labels []string) *types.Metric {
	for _, m := range metrics {
		if m.Name == name && reflect.DeepEqual(m.LabelValues, append(values, labels...)) {
			return m
		}
	}
	return nil
}
