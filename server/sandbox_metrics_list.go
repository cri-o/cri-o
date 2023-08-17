package server

import (
	"golang.org/x/net/context"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// ListPodSandboxMetrics lists all pod sandbox metrics.
func (s *Server) ListPodSandboxMetrics(ctx context.Context, req *types.ListPodSandboxRequest) (*types.ListPodSandboxMetricsResponse, error) {
	sboxList := s.ContainerServer.ListSandboxes()
	if req.Filter != nil {
		sbFilter := &types.PodSandboxFilter{
			Id:            req.Filter.Id,
			LabelSelector: req.Filter.LabelSelector,
		}
		sboxList = s.filterSandboxList(ctx, sbFilter, sboxList)
	}

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
