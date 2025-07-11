package server

import (
	"context"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// ListPodSandboxStats returns stats of all sandboxes.
func (s *Server) ListPodSandboxStats(ctx context.Context, req *types.ListPodSandboxStatsRequest) (*types.ListPodSandboxStatsResponse, error) {
	sboxList := s.ListSandboxes()

	if req.GetFilter() != nil {
		sbFilter := &types.PodSandboxFilter{
			Id:            req.GetFilter().GetId(),
			LabelSelector: req.GetFilter().GetLabelSelector(),
		}
		sboxList = s.filterSandboxList(ctx, sbFilter, sboxList)
	}

	return &types.ListPodSandboxStatsResponse{
		Stats: s.StatsForSandboxes(sboxList),
	}, nil
}
