package statsserver

import (
    types "k8s.io/cri-api/pkg/apis/runtime/v1"
    "github.com/cri-o/cri-o/internal/oci"
)

// metricValue, metricValues, and containerMetric are defined in metrics.go and should not be redeclared here.

// generateContainerTasksStateMetrics generates metrics for containerTasksState.
func generateContainerTasksStateMetrics(c *oci.Container) []*types.Metric {
    // getContainerTaskStates should return a map[string]uint64, e.g. {"sleeping": 3, "running": 1, ...}
    taskStates := getContainerTaskStates(c)

    metrics := []*containerMetric{
        {
            desc: containerTasksState,
            valueFunc: func() metricValues {
                var vals metricValues
                for _, v := range taskStates {
                    vals = append(vals, metricValue{
                        value:      v,
                        labels:     nil, // No extra labels, only baseLabelKeys
                        metricType: types.MetricType_GAUGE,
                    })
                }
                return vals
            },
        },
    }

    return computeContainerMetrics(c, metrics, "cpu_load")
}

// Helper to convert containerMetric to []*types.Metric (similar to computeSandboxMetrics)
func computeContainerMetrics(c *oci.Container, metrics []*containerMetric, metricType string) []*types.Metric {
    var result []*types.Metric
    baseLabels := getBaseLabelValues(c)
    for _, m := range metrics {
        for _, v := range m.valueFunc() {
            result = append(result, &types.Metric{
                Name:        m.desc.Name,
                LabelValues: append(baseLabels, v.labels...),
                Value:       &types.UInt64Value{Value: v.value},
            })
        }
    }
    return result
}

// getBaseLabelValues returns the base label values for a container.
// This is a stub implementation; adjust as needed for your metrics.
func getBaseLabelValues(c *oci.Container) []string {
    // Example: return container ID and name as labels
    return []string{c.ID(), c.Name()}
}

// getContainerTaskStates returns a map of task states for the given container.
// This is a stub implementation; replace with actual logic as needed.
func getContainerTaskStates(c *oci.Container) map[string]uint64 {
    // Example stub: return a fixed map for demonstration.
    return map[string]uint64{
        "sleeping": 2,
        "running":  1,
        "stopped":  0,
    }
}
