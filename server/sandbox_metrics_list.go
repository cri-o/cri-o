package server

import (
	"golang.org/x/net/context"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// ListPodSandboxMetrics lists all pod sandbox metrics.
func (s *Server) ListPodSandboxMetrics(ctx context.Context, req *types.ListPodSandboxMetricsRequest) (*types.ListPodSandboxMetricsResponse, error) {
	sboxList := s.ContainerServer.ListSandboxes()
	metricsList := s.ContainerServer.StatsForSandboxMetricsList(sboxList)
	responseMetricsList := make([]*types.PodSandboxMetrics, 0, len(metricsList))

	for _, metrics := range metricsList {
		if current := metrics.GetCurrent(); current != nil {
			responseMetricsList = append(responseMetricsList, current)
		}
	}
	return &types.ListPodSandboxMetricsResponse{
		PodMetrics: responseMetricsList,
	}, nil
}
