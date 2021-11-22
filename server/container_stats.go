package server

import (
	"context"

	"github.com/pkg/errors"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// ContainerStats returns stats of the container. If the container does not
// exist, the call returns an error.
func (s *Server) ContainerStats(ctx context.Context, req *types.ContainerStatsRequest) (*types.ContainerStatsResponse, error) {
	container, err := s.GetContainerFromShortID(req.ContainerId)
	if err != nil {
		return nil, err
	}
	sb := s.GetSandbox(container.Sandbox())
	if sb == nil {
		return nil, errors.Errorf("unable to get stats for container %s: sandbox %s not found", container.ID(), container.Sandbox())
	}

	return &types.ContainerStatsResponse{
		Stats: s.ContainerServer.StatsForContainer(container, sb),
	}, nil
}
