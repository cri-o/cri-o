package v1

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) ExecSync(
	ctx context.Context, req *pb.ExecSyncRequest,
) (*pb.ExecSyncResponse, error) {
	r := &types.ExecSyncRequest{
		ContainerID: req.ContainerId,
		Cmd:         req.Cmd,
		Timeout:     req.Timeout,
	}
	res, err := s.server.ExecSync(ctx, r)
	if err != nil {
		return nil, err
	}
	return &pb.ExecSyncResponse{
		Stdout:   res.Stdout,
		Stderr:   res.Stderr,
		ExitCode: res.ExitCode,
	}, nil
}
