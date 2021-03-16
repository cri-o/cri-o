package server

import (
	"github.com/cri-o/cri-o/server/cri/types"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ExecSync runs a command in a container synchronously.
func (s *Server) ExecSync(ctx context.Context, req *types.ExecSyncRequest) (*types.ExecSyncResponse, error) {
	c, err := s.GetContainerFromShortID(req.ContainerID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "could not find container %q: %v", req.ContainerID, err)
	}

	if err := c.IsAlive(); err != nil {
		return nil, status.Errorf(codes.NotFound, "container is not created or running: %v", err)
	}

	cmd := req.Cmd
	if cmd == nil {
		return nil, errors.New("exec command cannot be empty")
	}

	execResp, err := s.Runtime().ExecSyncContainer(ctx, c, cmd, req.Timeout)
	if err != nil {
		return nil, err
	}
	return &types.ExecSyncResponse{
		Stdout:   execResp.Stdout,
		Stderr:   execResp.Stderr,
		ExitCode: execResp.ExitCode,
	}, nil
}
