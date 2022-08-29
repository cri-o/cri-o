package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) CheckpointContainer(ctx context.Context, req *pb.CheckpointContainerRequest) (*pb.CheckpointContainerResponse, error) {
	result := s.server.CheckpointContainer(ctx, req)

	return &pb.CheckpointContainerResponse{}, result
}
