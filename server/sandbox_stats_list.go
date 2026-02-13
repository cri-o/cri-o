package server

import (
	"context"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// ListPodSandboxStats returns stats of all sandboxes.
func (s *Server) ListPodSandboxStats(ctx context.Context, req *types.ListPodSandboxStatsRequest) (*types.ListPodSandboxStatsResponse, error) {
	return &types.ListPodSandboxStatsResponse{
		Stats: s.listPodSandboxStats(ctx, req.GetFilter()),
	}, nil
}

// StreamPodSandboxStats returns a stream of pod sandbox stats.
func (s *Server) StreamPodSandboxStats(req *types.StreamPodSandboxStatsRequest, stream types.RuntimeService_StreamPodSandboxStatsServer) error {
	ctx := stream.Context()

	for _, stat := range s.listPodSandboxStats(ctx, req.GetFilter()) {
		if err := stream.Send(&types.StreamPodSandboxStatsResponse{
			PodSandboxStats: stat,
		}); err != nil {
			return err
		}
	}

	return nil
}

// listPodSandboxStats returns stats for sandboxes matching the filter.
func (s *Server) listPodSandboxStats(ctx context.Context, filter *types.PodSandboxStatsFilter) []*types.PodSandboxStats {
	sboxList := s.ListSandboxes()

	if filter != nil {
		sbFilter := &types.PodSandboxFilter{
			Id:            filter.GetId(),
			LabelSelector: filter.GetLabelSelector(),
		}
		sboxList = s.filterSandboxList(ctx, sbFilter, sboxList)
	}

	return s.StatsForSandboxes(sboxList)
}
