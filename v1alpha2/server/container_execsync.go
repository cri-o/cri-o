package server

import (
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// ExecSync runs a command in a container synchronously.
func (s *Server) ExecSync(ctx context.Context, req *pb.ExecSyncRequest) (*pb.ExecSyncResponse, error) {
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

	execResp, err := s.Runtime().ExecSyncContainer(c, cmd, req.Timeout)
	if err != nil {
		return nil, err
	}
	return &pb.ExecSyncResponse{
		Stdout:   execResp.Stdout,
		Stderr:   execResp.Stderr,
		ExitCode: execResp.ExitCode,
	}, nil
}
