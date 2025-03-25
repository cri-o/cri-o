package server

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/log"
)

// ExecSync runs a command in a container synchronously.
func (s *Server) ExecSync(ctx context.Context, req *types.ExecSyncRequest) (*types.ExecSyncResponse, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	c, err := s.GetContainerFromShortID(ctx, req.ContainerId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "could not find container %q: %v", req.ContainerId, err)
	}

	if err := c.Living(); err != nil {
		return nil, status.Errorf(codes.NotFound, "container is not created or running: %v", err)
	}

	cmd := req.Cmd
	if cmd == nil {
		return nil, errors.New("exec command cannot be empty")
	}

	return s.ContainerServer.Runtime().ExecSyncContainer(ctx, c, cmd, req.Timeout)
}
