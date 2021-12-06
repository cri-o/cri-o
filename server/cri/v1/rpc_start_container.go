package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) StartContainer(
	ctx context.Context, req *pb.StartContainerRequest,
) (resp *pb.StartContainerResponse, retErr error) {
	if err := s.server.StartContainer(ctx, req); err != nil {
		return nil, err
	}
	return &pb.StartContainerResponse{}, nil
}
