package v1

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) Exec(
	ctx context.Context, req *pb.ExecRequest,
) (*pb.ExecResponse, error) {
	r := &types.ExecRequest{
		ContainerID: req.ContainerId,
		Cmd:         req.Cmd,
		Tty:         req.Tty,
		Stdin:       req.Stdin,
		Stdout:      req.Stdout,
		Stderr:      req.Stderr,
	}
	res, err := s.server.Exec(ctx, r)
	if err != nil {
		return nil, err
	}
	return &pb.ExecResponse{Url: res.URL}, nil
}
