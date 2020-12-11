package v1alpha2

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) Attach(
	ctx context.Context, req *pb.AttachRequest,
) (*pb.AttachResponse, error) {
	r := &types.AttachRequest{
		ContainerID: req.ContainerId,
		Stdin:       req.Stdin,
		Tty:         req.Tty,
		Stdout:      req.Stdout,
		Stderr:      req.Stderr,
	}
	res, err := s.server.Attach(ctx, r)
	if err != nil {
		return nil, err
	}
	return &pb.AttachResponse{Url: res.URL}, nil
}
