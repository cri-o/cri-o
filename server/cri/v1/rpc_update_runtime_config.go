package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) UpdateRuntimeConfig(
	ctx context.Context, req *pb.UpdateRuntimeConfigRequest,
) (*pb.UpdateRuntimeConfigResponse, error) {
	return &pb.UpdateRuntimeConfigResponse{}, nil
}
