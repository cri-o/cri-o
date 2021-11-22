package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) StopContainer(
	ctx context.Context, req *pb.StopContainerRequest,
) (*pb.StopContainerResponse, error) {
	if err := s.server.StopContainer(ctx, req); err != nil {
		return nil, err
	}
	return &pb.StopContainerResponse{}, nil
}
