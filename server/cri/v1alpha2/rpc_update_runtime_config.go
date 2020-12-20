package v1alpha2

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) UpdateRuntimeConfig(
	ctx context.Context, req *pb.UpdateRuntimeConfigRequest,
) (*pb.UpdateRuntimeConfigResponse, error) {
	return &pb.UpdateRuntimeConfigResponse{}, nil
}
