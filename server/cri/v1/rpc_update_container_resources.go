package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) UpdateContainerResources(
	ctx context.Context, req *pb.UpdateContainerResourcesRequest,
) (*pb.UpdateContainerResourcesResponse, error) {
	if err := s.server.UpdateContainerResources(ctx, req); err != nil {
		return nil, err
	}
	return &pb.UpdateContainerResourcesResponse{}, nil
}
