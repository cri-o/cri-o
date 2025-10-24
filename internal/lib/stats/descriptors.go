package statsserver

import types "k8s.io/cri-api/pkg/apis/runtime/v1"

// CPU metrics.
var (
	containerCpuUserSecondsTotal = &types.MetricDescriptor{
		Name:      "container_cpu_user_seconds_total",
		Help:      "Cumulative user CPU time consumed in seconds.",
		LabelKeys: baseLabelKeys,
	}
	containerCpuSystemSecondsTotal = &types.MetricDescriptor{
		Name:      "container_cpu_system_seconds_total",
		Help:      "Cumulative system CPU time consumed in seconds.",
		LabelKeys: baseLabelKeys,
	}
	containerCpuUsageSecondsTotal = &types.MetricDescriptor{
		Name:      "container_cpu_usage_seconds_total",
		Help:      "Cumulative CPU time consumed in seconds.",
		LabelKeys: append(baseLabelKeys, "cpu"),
	}
	containerCpuCfsPeriodsTotal = &types.MetricDescriptor{
		Name:      "container_cpu_cfs_periods_total",
		Help:      "Number of elapsed enforcement period intervals.",
		LabelKeys: baseLabelKeys,
	}
	containerCpuCfsThrottledPeriodsTotal = &types.MetricDescriptor{
		Name:      "container_cpu_cfs_throttled_periods_total",
		Help:      "Number of throttled period intervals.",
		LabelKeys: baseLabelKeys,
	}
	containerCpuCfsThrottledSecondsTotal = &types.MetricDescriptor{
		Name:      "container_cpu_cfs_throttled_seconds_total",
		Help:      "Total time duration the container has been throttled.",
		LabelKeys: baseLabelKeys,
	}
)

// HugeTLB metrics.
var (
	containerHugetlbUsageBytes = &types.MetricDescriptor{
		Name:      "container_hugetlb_usage_bytes",
		Help:      "Current hugepage usage",
		LabelKeys: append(baseLabelKeys, "pagesize"),
	}
	containerHugetlbMaxUsageBytes = &types.MetricDescriptor{
		Name:      "container_hugetlb_max_usage_bytes",
		Help:      "Maximum hugepage usages recorded",
		LabelKeys: append(baseLabelKeys, "pagesize"),
	}
)

// Memory metrics.
var (
	containerMemoryCache = &types.MetricDescriptor{
		Name:      "container_memory_cache",
		Help:      "Number of bytes of page cache memory.",
		LabelKeys: baseLabelKeys,
	}
	containerMemoryRss = &types.MetricDescriptor{
		Name:      "container_memory_rss",
		Help:      "Size of RSS in bytes.",
		LabelKeys: baseLabelKeys,
	}
	containerMemoryKernelUsage = &types.MetricDescriptor{
		Name:      "container_memory_kernel_usage",
		Help:      "Size of kernel memory allocated in bytes.",
		LabelKeys: baseLabelKeys,
	}
	containerMemoryMappedFile = &types.MetricDescriptor{
		Name:      "container_memory_mapped_file",
		Help:      "Size of memory mapped files in bytes.",
		LabelKeys: baseLabelKeys,
	}
	containerMemorySwap = &types.MetricDescriptor{
		Name:      "container_memory_swap",
		Help:      "Container swap usage in bytes.",
		LabelKeys: baseLabelKeys,
	}
	containerMemoryFailcnt = &types.MetricDescriptor{
		Name:      "container_memory_failcnt",
		Help:      "Number of memory usage hits limits",
		LabelKeys: baseLabelKeys,
	}
	containerMemoryUsageBytes = &types.MetricDescriptor{
		Name:      "container_memory_usage_bytes",
		Help:      "Current memory usage in bytes, including all memory regardless of when it was accessed",
		LabelKeys: baseLabelKeys,
	}
	containerMemoryMaxUsageBytes = &types.MetricDescriptor{
		Name:      "container_memory_max_usage_bytes",
		Help:      "Maximum memory usage recorded in bytes",
		LabelKeys: baseLabelKeys,
	}
	containerMemoryWorkingSetBytes = &types.MetricDescriptor{
		Name:      "container_memory_working_set_bytes",
		Help:      "Current working set in bytes.",
		LabelKeys: baseLabelKeys,
	}
	containerMemoryFailuresTotal = &types.MetricDescriptor{
		Name:      "container_memory_failures_total",
		Help:      "Cumulative count of memory allocation failures.",
		LabelKeys: append(baseLabelKeys, "failure_type", "scope"),
	}
)

var networkLabelKeys = append(baseLabelKeys, "interface")

// Network metrics.
var (
	containerNetworkReceiveBytesTotal = &types.MetricDescriptor{
		Name:      "container_network_receive_bytes_total",
		Help:      "Cumulative count of bytes received",
		LabelKeys: networkLabelKeys,
	}
	containerNetworkReceivePacketsTotal = &types.MetricDescriptor{
		Name:      "container_network_receive_packets_total",
		Help:      "Cumulative count of packets received",
		LabelKeys: networkLabelKeys,
	}
	containerNetworkReceivePacketsDroppedTotal = &types.MetricDescriptor{
		Name:      "container_network_receive_packets_dropped_total",
		Help:      "Cumulative count of packets dropped while receiving",
		LabelKeys: networkLabelKeys,
	}
	containerNetworkReceiveErrorsTotal = &types.MetricDescriptor{
		Name:      "container_network_receive_errors_total",
		Help:      "Cumulative count of errors encountered while receiving",
		LabelKeys: networkLabelKeys,
	}
	containerNetworkTransmitBytesTotal = &types.MetricDescriptor{
		Name:      "container_network_transmit_bytes_total",
		Help:      "Cumulative count of bytes transmitted",
		LabelKeys: networkLabelKeys,
	}
	containerNetworkTransmitPacketsTotal = &types.MetricDescriptor{
		Name:      "container_network_transmit_packets_total",
		Help:      "Cumulative count of packets transmitted",
		LabelKeys: networkLabelKeys,
	}
	containerNetworkTransmitPacketsDroppedTotal = &types.MetricDescriptor{
		Name:      "container_network_transmit_packets_dropped_total",
		Help:      "Cumulative count of packets dropped while transmitting",
		LabelKeys: networkLabelKeys,
	}
	containerNetworkTransmitErrorsTotal = &types.MetricDescriptor{
		Name:      "container_network_transmit_errors_total",
		Help:      "Cumulative count of errors encountered while transmitting",
		LabelKeys: networkLabelKeys,
	}
)

// OOM metrics.
var (
	containerOomEventsTotal = &types.MetricDescriptor{
		Name:      "container_oom_events_total",
		Help:      "Count of out of memory events observed for the container",
		LabelKeys: baseLabelKeys,
	}
)

// Process metrics.
var (
	containerProcesses = &types.MetricDescriptor{
		Name:      "container_processes",
		Help:      "Number of processes running inside the container",
		LabelKeys: baseLabelKeys,
	}
)

// Miscellaneous metrics.
var (
	containerLastSeen = &types.MetricDescriptor{
		Name:      "container_last_seen",
		Help:      "Last time a container was seen by the exporter",
		LabelKeys: baseLabelKeys,
	}
)

// Spec metrics.
var (
	containerSpecMemoryLimitBytes = &types.MetricDescriptor{
		Name:      "container_spec_memory_limit_bytes",
		Help:      "Memory limit for the container in bytes.",
		LabelKeys: baseLabelKeys,
	}
	containerSpecCpuPeriod = &types.MetricDescriptor{
		Name:      "container_spec_cpu_period",
		Help:      "CPU period of the container.",
		LabelKeys: baseLabelKeys,
	}
	containerSpecCpuShares = &types.MetricDescriptor{
		Name:      "container_spec_cpu_shares",
		Help:      "CPU share of the container.",
		LabelKeys: baseLabelKeys,
	}
	containerSpecCpuQuota = &types.MetricDescriptor{
		Name:      "container_spec_cpu_quota",
		Help:      "CPU quota of the container.",
		LabelKeys: baseLabelKeys,
	}
	containerSpecMemoryReservationLimitBytes = &types.MetricDescriptor{
		Name:      "container_spec_memory_reservation_limit_bytes",
		Help:      "Memory reservation limit for the container.",
		LabelKeys: baseLabelKeys,
	}
	containerSpecMemorySwapLimitBytes = &types.MetricDescriptor{
		Name:      "container_spec_memory_swap_limit_bytes",
		Help:      "Memory swap limit for the container.",
		LabelKeys: baseLabelKeys,
	}
)
