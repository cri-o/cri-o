package server

import (
	"context"
	"fmt"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/log"
)

// ContainerStats returns stats of the container. If the container does not
// exist, the call returns an error.
func (s *Server) ContainerStats(ctx context.Context, req *types.ContainerStatsRequest) (*types.ContainerStatsResponse, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	container, err := s.GetContainerFromShortID(ctx, req.ContainerId)
	if err != nil {
		return nil, err
	}
	sb := s.GetSandbox(container.Sandbox())
	if sb == nil {
		return nil, fmt.Errorf("unable to get stats for container %s: sandbox %s not found", container.ID(), container.Sandbox())
	}

	return &types.ContainerStatsResponse{
		Stats: s.ContainerServer.StatsForContainer(container, sb),
	}, nil
}
