//go:build linux

package statsserver

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/opencontainers/cgroups"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/lib/stats"
	"github.com/cri-o/cri-o/internal/oci"
)

// generateContainerDiskMetrics computes filesystem disk metrics from DiskStats for a container sandbox.
func generateContainerDiskMetrics(ctr *oci.Container, diskStats *stats.FilesystemStats) []*types.Metric {
	if diskStats == nil {
		return []*types.Metric{}
	}

	diskMetrics := []*containerMetric{
		{
			desc: containerFsInodesFree,
			valueFunc: func() metricValues {
				return metricValues{{value: diskStats.InodesFree, metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: containerFsInodesTotal,
			valueFunc: func() metricValues {
				return metricValues{{value: diskStats.InodesTotal, metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: containerFsLimitBytes,
			valueFunc: func() metricValues {
				return metricValues{{value: diskStats.LimitBytes, metricType: types.MetricType_GAUGE}}
			},
		},
		{
			desc: containerFsUsageBytes,
			valueFunc: func() metricValues {
				return metricValues{{value: diskStats.UsageBytes, metricType: types.MetricType_GAUGE}}
			},
		},
	}

	return computeContainerMetrics(ctr, diskMetrics, "disk")
}

// generateContainerDiskIOMetrics computes filesystem disk metrics from DiskStats for a container sandbox.
func generateContainerDiskIOMetrics(ctr *oci.Container, ioStats *cgroups.BlkioStats) []*types.Metric {
	if ioStats == nil {
		return []*types.Metric{}
	}

	diskMetrics := []*containerMetric{}

	for _, stat := range ioStats.IoServicedRecursive {
		// TODO (@haircommander): cadvisor translates to device name, but finding the device is tricky
		// update to populate device name
		device := fmt.Sprintf("%d:%d", stat.Major, stat.Minor)
		switch stat.Op {
		case "Write":
			diskMetrics = append(diskMetrics, &containerMetric{
				desc: containerFsWritesTotal,
				valueFunc: func() metricValues {
					return metricValues{{value: stat.Value, labels: []string{device}, metricType: types.MetricType_COUNTER}}
				},
			})
		case "Read":
			diskMetrics = append(diskMetrics, &containerMetric{
				desc: containerFsReadsTotal,
				valueFunc: func() metricValues {
					return metricValues{{value: stat.Value, labels: []string{device}, metricType: types.MetricType_COUNTER}}
				},
			})
		}
	}

	for _, stat := range ioStats.IoServiceBytesRecursive {
		// TODO (@haircommander): cadvisor translates to device name, but finding the device is tricky
		// update to populate device name
		device := fmt.Sprintf("%d:%d", stat.Major, stat.Minor)
		deviceLabels := []string{device, strconv.FormatUint(stat.Major, 10), strconv.FormatUint(stat.Minor, 10), strings.ToLower(stat.Op)}

		diskMetrics = append(diskMetrics, &containerMetric{
			desc: containerBlkioDeviceUsageTotal,
			valueFunc: func() metricValues {
				return metricValues{{value: stat.Value, labels: deviceLabels, metricType: types.MetricType_COUNTER}}
			},
		})

		switch stat.Op {
		case "Write":
			diskMetrics = append(diskMetrics, &containerMetric{
				desc: containerFsWritesBytesTotal,
				valueFunc: func() metricValues {
					return metricValues{{value: stat.Value, labels: []string{device}, metricType: types.MetricType_COUNTER}}
				},
			})
		case "Read":
			diskMetrics = append(diskMetrics, &containerMetric{
				desc: containerFsReadsBytesTotal,
				valueFunc: func() metricValues {
					return metricValues{{value: stat.Value, labels: []string{device}, metricType: types.MetricType_COUNTER}}
				},
			})
		}
	}

	return computeContainerMetrics(ctr, diskMetrics, "diskIO")
}
