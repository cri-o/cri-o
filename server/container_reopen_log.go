package server

import (
	"context"
	"errors"
	"fmt"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/log"
)

// ReopenContainerLog reopens the containers log file.
func (s *Server) ReopenContainerLog(ctx context.Context, req *types.ReopenContainerLogRequest) (*types.ReopenContainerLogResponse, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	c, err := s.ContainerServer.GetContainerFromShortID(ctx, req.ContainerId)
	if err != nil {
		return nil, fmt.Errorf("could not find container %s: %w", req.ContainerId, err)
	}

	isRunning, err := s.ContainerServer.Runtime().IsContainerAlive(c)
	if err != nil {
		return nil, err
	}

	if !isRunning {
		return nil, errors.New("container is not running")
	}

	if err := s.ContainerServer.Runtime().ReopenContainerLog(ctx, c); err != nil {
		return nil, err
	}

	return &types.ReopenContainerLogResponse{}, nil
}
