package v1

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) StopContainer(
	ctx context.Context, req *pb.StopContainerRequest,
) (*pb.StopContainerResponse, error) {
	r := &types.StopContainerRequest{
		ContainerID: req.ContainerId,
		Timeout:     req.Timeout,
	}
	if err := s.server.StopContainer(ctx, r); err != nil {
		return nil, err
	}
	return &pb.StopContainerResponse{}, nil
}
