package server

import (
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// ExecSync runs a command in a container synchronously.
func (s *Server) ExecSync(ctx context.Context, req *types.ExecSyncRequest) (*types.ExecSyncResponse, error) {
	c, err := s.GetContainerFromShortID(req.ContainerId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "could not find container %q: %v", req.ContainerId, err)
	}

	if err := c.IsAlive(); err != nil {
		return nil, status.Errorf(codes.NotFound, "container is not created or running: %v", err)
	}

	cmd := req.Cmd
	if cmd == nil {
		return nil, errors.New("exec command cannot be empty")
	}

	return s.Runtime().ExecSyncContainer(ctx, c, cmd, req.Timeout)
}
