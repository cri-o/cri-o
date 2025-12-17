//go:build linux

package statsserver

import (
	"fmt"
	"strconv"
	"strings"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/config/cgmgr"
	"github.com/cri-o/cri-o/internal/oci"
)

// generateSandboxDiskMetrics computes filesystem disk metrics from DiskMetrics for a container sandbox.
func generateContainerDiskMetrics(ctr *oci.Container, diskStats *oci.FilesystemMetrics) []*types.Metric {
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

// generateSandboxDiskIOMetrics computes filesystem disk metrics from DiskMetrics for a container sandbox.
func generateContainerDiskIOMetrics(ctr *oci.Container, ioStats *cgmgr.DiskIOStats) []*types.Metric {
	diskMetrics := []*containerMetric{}

	for _, stat := range ioStats.IoServiced {
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

	for _, stat := range ioStats.IoServiceBytes {
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
