//go:build test

// All *_inject.go files are meant to be used by tests only. Purpose of this
// files is to provide a way to inject mocked data into the current setup.

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// ContainersOOMCountTotal returns the OOM count recorded for the container
// name, and whether it is recorded at all.
func (m *Metrics) ContainersOOMCountTotal(name string) (count float64, ok bool) {
	ch := make(chan prometheus.Metric)

	go func() {
		m.metricContainersOOMCountTotal.Collect(ch)
		close(ch)
	}()

	for metric := range ch {
		var out dto.Metric
		if err := metric.Write(&out); err != nil {
			continue
		}

		for _, label := range out.GetLabel() {
			if label.GetName() == "name" && label.GetValue() == name {
				count, ok = out.GetCounter().GetValue(), true
			}
		}
	}

	return count, ok
}
