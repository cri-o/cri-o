package v1

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) RemoveContainer(
	ctx context.Context, req *pb.RemoveContainerRequest,
) (*pb.RemoveContainerResponse, error) {
	r := &types.RemoveContainerRequest{
		ContainerID: req.ContainerId,
	}
	if err := s.server.RemoveContainer(ctx, r); err != nil {
		return nil, err
	}
	return &pb.RemoveContainerResponse{}, nil
}
