package server

import (
	"context"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// ListPodSandboxStats returns stats of all sandboxes.
func (s *Server) ListPodSandboxStats(ctx context.Context, req *types.ListPodSandboxStatsRequest) (*types.ListPodSandboxStatsResponse, error) {
	sboxList := s.ContainerServer.ListSandboxes()
	if req.Filter != nil {
		sbFilter := &types.PodSandboxFilter{
			Id:            req.Filter.Id,
			LabelSelector: req.Filter.LabelSelector,
		}
		sboxList = s.filterSandboxList(ctx, sbFilter, sboxList)
	}

	return &types.ListPodSandboxStatsResponse{
		Stats: s.ContainerServer.StatsForSandboxes(sboxList),
	}, nil
}
