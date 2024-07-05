package server

import (
	"context"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/log"
)

// PodSandboxStats returns stats of the sandbox. If the sandbox does not exist, the call returns an error.
func (s *Server) PodSandboxStats(ctx context.Context, req *types.PodSandboxStatsRequest) (*types.PodSandboxStatsResponse, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	sb, err := s.getPodSandboxFromRequest(ctx, req.PodSandboxId)
	if err != nil {
		return nil, err
	}

	return &types.PodSandboxStatsResponse{
		Stats: s.ContainerServer.StatsForSandbox(sb),
	}, nil
}
