package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) RemoveContainer(
	ctx context.Context, req *pb.RemoveContainerRequest,
) (*pb.RemoveContainerResponse, error) {
	if err := s.server.RemoveContainer(ctx, req); err != nil {
		return nil, err
	}
	return &pb.RemoveContainerResponse{}, nil
}
