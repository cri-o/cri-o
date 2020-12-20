package v1alpha2

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) StartContainer(
	ctx context.Context, req *pb.StartContainerRequest,
) (resp *pb.StartContainerResponse, retErr error) {
	r := &types.StartContainerRequest{
		ContainerID: req.ContainerId,
	}
	if err := s.server.StartContainer(ctx, r); err != nil {
		return nil, err
	}
	return &pb.StartContainerResponse{}, nil
}
