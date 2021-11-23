package server

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
)

// PodSandboxStats returns stats of the sandbox. If the sandbox does not exist, the call returns an error.
func (s *Server) PodSandboxStats(ctx context.Context, req *types.PodSandboxStatsRequest) (*types.PodSandboxStatsResponse, error) {
	sb, err := s.getPodSandboxFromRequest(req.PodSandboxID)
	if err != nil {
		return nil, err
	}

	return &types.PodSandboxStatsResponse{
		Stats: s.ContainerServer.StatsForSandbox(sb),
	}, nil
}
