package server

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
)

// ListPodSandboxStats returns stats of all sandboxes.
func (s *Server) ListPodSandboxStats(ctx context.Context, req *types.ListPodSandboxStatsRequest) (*types.ListPodSandboxStatsResponse, error) {
	sboxList := s.ContainerServer.ListSandboxes()
	if req.Filter != nil {
		sbFilter := &types.PodSandboxFilter{
			ID:            req.Filter.ID,
			LabelSelector: req.Filter.LabelSelector,
		}
		sboxList = s.filterSandboxList(ctx, sbFilter, sboxList)
	}

	return &types.ListPodSandboxStatsResponse{
		Stats: s.ContainerServer.StatsForSandboxes(sboxList),
	}, nil
}
